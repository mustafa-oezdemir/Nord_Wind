package inventory

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

func TestReservationsOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_INVENTORY_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_INVENTORY_TEST_URL für eine separate Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Separate Testdatenbank erforderlich: %q, %v", name, err)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM orders`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Leere Testdatenbank erforderlich: orders=%d, %v", count, err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`TRUNCATE order_status_events, order_reservations, order_items,
			orders, product_variants, products, customers CASCADE`); err != nil {
			t.Errorf("Cleanup: %v", err)
		}
	})
	var customerID, productID, variantID string
	if err := db.QueryRowContext(ctx, `INSERT INTO customers (email) VALUES ('inventory@example.invalid') RETURNING id`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO products (name, slug, status) VALUES ('Reservierung', 'reservierung', 'published') RETURNING id`).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO product_variants (product_id, sku, label, price_cents, stock_quantity)
		VALUES ($1, 'NW-INVENTORY', 'Standard', 100, 1) RETURNING id`, productID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	newOrder := func(number string) string {
		t.Helper()
		var id string
		err := db.QueryRowContext(ctx, `INSERT INTO orders
			(order_number, customer_id, status, total_cents, shipping_snapshot,
			idempotency_key, request_hash, customer_email, goods_cents, discount_cents,
			shipping_cents, tax_lines)
			VALUES ($1, $2, 'PAYMENT_PENDING', 100, '{"address":{}}', $3,
			decode(repeat('ab', 32), 'hex'), 'inventory@example.invalid', 100, 0, 0, '[{}]') RETURNING id`,
			number, customerID, number+"-checkout-key").Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	orders := []string{newOrder("NW-INVENTORY-A"), newOrder("NW-INVENTORY-B")}
	results := make([]error, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := range orders {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			results[index] = WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
				return Reserve(ctx, tx, orders[index], []Line{{VariantID: variantID, Quantity: 1}}, time.Now().Add(time.Minute))
			})
		}(index)
	}
	close(start)
	group.Wait()
	winner := -1
	for index, result := range results {
		if result == nil {
			if winner != -1 {
				t.Fatalf("Zwei Reservierungen für ein Stück erfolgreich: %v", results)
			}
			winner = index
		} else if !errors.Is(result, ErrUnavailable) {
			t.Fatalf("Unerwarteter Checkout-Fehler: %v", result)
		}
	}
	if winner < 0 {
		t.Fatalf("Keine Reservierung erfolgreich: %v", results)
	}
	assertStock := func(want int) {
		t.Helper()
		var stock int
		if err := db.QueryRowContext(ctx, `SELECT stock_quantity FROM product_variants WHERE id = $1`, variantID).Scan(&stock); err != nil || stock != want {
			t.Fatalf("Bestand %d statt %d: %v", stock, want, err)
		}
	}
	assertStock(0)
	if _, err := db.ExecContext(ctx, `UPDATE order_reservations SET reserved_at = now() - interval '2 minutes',
		expires_at = now() - interval '1 second'
		WHERE order_id = $1`, orders[winner]); err != nil {
		t.Fatal(err)
	}
	for index, want := range []int{1, 0} {
		released, err := ReleaseExpired(ctx, db, time.Now(), 10)
		if err != nil || released != want {
			t.Fatalf("Ablauf %d: %d statt %d, %v", index, released, want, err)
		}
	}
	assertStock(1)
	cancelOrder := newOrder("NW-INVENTORY-C")
	if err := WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
		return Reserve(ctx, tx, cancelOrder, []Line{{VariantID: variantID, Quantity: 1}}, time.Now().Add(time.Minute))
	}); err != nil {
		t.Fatal(err)
	}
	assertStock(0)
	if err := Cancel(ctx, db, cancelOrder); err != nil {
		t.Fatal(err)
	}
	if err := Cancel(ctx, db, cancelOrder); err != nil {
		t.Fatalf("Wiederholte Stornierung: %v", err)
	}
	assertStock(1)
	paidOrder := newOrder("NW-INVENTORY-D")
	if err := WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
		return Reserve(ctx, tx, paidOrder, []Line{{VariantID: variantID, Quantity: 1}}, time.Now().Add(time.Minute))
	}); err != nil {
		t.Fatal(err)
	}
	if err := WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
		if err := Consume(ctx, tx, paidOrder); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'PAID' WHERE id = $1`, paidOrder)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := Cancel(ctx, db, paidOrder); !errors.Is(err, ErrState) {
		t.Fatalf("Bezahlte Bestellung stornierbar: %v", err)
	}
	assertStock(0)
	var extraIDs [2]string
	for index, sku := range []string{"NW-ROLLBACK-A", "NW-ROLLBACK-B"} {
		if err := db.QueryRowContext(ctx, `INSERT INTO product_variants
			(product_id, sku, label, price_cents, stock_quantity)
			VALUES ($1, $2, 'Rollback', 100, 0) RETURNING id`, productID, sku).Scan(&extraIDs[index]); err != nil {
			t.Fatal(err)
		}
	}
	if extraIDs[1] < extraIDs[0] {
		extraIDs[0], extraIDs[1] = extraIDs[1], extraIDs[0]
	}
	if _, err := db.ExecContext(ctx, `UPDATE product_variants SET stock_quantity = 1 WHERE id = $1`, extraIDs[0]); err != nil {
		t.Fatal(err)
	}
	rollbackOrder := newOrder("NW-INVENTORY-ROLLBACK")
	err = WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
		return Reserve(ctx, tx, rollbackOrder,
			[]Line{{VariantID: extraIDs[1], Quantity: 1}, {VariantID: extraIDs[0], Quantity: 1}},
			time.Now().Add(time.Minute))
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Teilweise Reservierung wurde nicht abgebrochen: %v", err)
	}
	var rollbackStock, rollbackReservations int
	if err := db.QueryRowContext(ctx, `SELECT stock_quantity FROM product_variants WHERE id = $1`, extraIDs[0]).Scan(&rollbackStock); err != nil || rollbackStock != 1 {
		t.Fatalf("Teilweiser Bestandsabzug nicht zurückgerollt: %d, %v", rollbackStock, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM order_reservations WHERE order_id = $1`, rollbackOrder).Scan(&rollbackReservations); err != nil || rollbackReservations != 0 {
		t.Fatalf("Teilweise Reservierung gespeichert: %d, %v", rollbackReservations, err)
	}
	attempts := 0
	if err := WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
		attempts++
		if attempts == 1 {
			return &pgconn.PgError{Code: "40P01"}
		}
		_, err := tx.ExecContext(ctx, `UPDATE products SET updated_at = now() WHERE id = $1`, productID)
		return err
	}); err != nil || attempts != 2 {
		t.Fatalf("Deadlock-Retry: Versuche=%d, Fehler=%v", attempts, err)
	}
	// Zwei Transaktionen greifen absichtlich in entgegengesetzter Reihenfolge auf
	// Advisory Locks zu. PostgreSQL muss einen echten 40P01 liefern; der Helfer
	// rollt diesen Versuch zurück und startet die Transaktion neu.
	firstLocks := make(chan struct{}, 2)
	bothLocked := make(chan struct{})
	go func() {
		<-firstLocks
		<-firstLocks
		close(bothLocked)
	}()
	deadlockAttempts := [2]int{}
	deadlockResults := [2]error{}
	for index := 0; index < 2; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			first, second := 912001, 912002
			if index == 1 {
				first, second = second, first
			}
			deadlockResults[index] = WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
				deadlockAttempts[index]++
				if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, first); err != nil {
					return err
				}
				if deadlockAttempts[index] == 1 {
					firstLocks <- struct{}{}
					<-bothLocked
				}
				_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, second)
				return err
			})
		}(index)
	}
	group.Wait()
	if deadlockResults[0] != nil || deadlockResults[1] != nil ||
		(deadlockAttempts[0] < 2 && deadlockAttempts[1] < 2) {
		t.Fatalf("Echter Deadlock nicht wiederholt: Versuche=%v, Fehler=%v", deadlockAttempts, deadlockResults)
	}
}
