package server

import (
	"context"
	"database/sql"
	"time"
)

// Geschäfts- und Prozesskennzahlen werden bei jedem Abruf aus PostgreSQL gelesen.
// So stimmen sie auch nach Neustarts und mit mehreren API-Instanzen. Es werden nur
// Anzahlen und Summen gelesen, nie Namen, Adressen oder Bestellnummern.
const businessMetricsQuery = `
SELECT 'orders', status, count(*)::float8, coalesce(sum(total_cents), 0)::float8 / 100
  FROM orders GROUP BY status
UNION ALL
SELECT 'customers', CASE WHEN anonymized_at IS NULL THEN 'active' ELSE 'closed' END, count(*)::float8, 0::float8
  FROM customers WHERE role = 'customer' GROUP BY 2
UNION ALL
SELECT 'payments', state, count(*)::float8, 0::float8 FROM mock_payments GROUP BY state
UNION ALL
SELECT 'outbox', state, count(*)::float8,
       CASE WHEN state IN ('pending', 'sending') THEN extract(epoch FROM now() - min(created_at))::float8 ELSE 0::float8 END
  FROM notification_outbox GROUP BY state
UNION ALL
SELECT 'reservations', 'active', count(*)::float8,
       (count(*) FILTER (WHERE expires_at < now() - interval '5 minutes'))::float8
  FROM order_reservations WHERE state = 'active'
UNION ALL
SELECT 'stock', 'out_of_stock', count(*)::float8, 0::float8
  FROM product_variants v JOIN products p ON p.id = v.product_id
 WHERE p.status = 'published' AND v.is_active AND v.stock_quantity = 0`

var (
	orderStatuses  = []string{"CREATED", "PAYMENT_PENDING", "PAID", "PROCESSING", "SHIPPED", "DELIVERED", "CANCELLED", "PAYMENT_FAILED"}
	customerStates = []string{"active", "closed"}
	paymentStates  = []string{"pending", "succeeded", "failed", "refunded"}
	outboxStates   = []string{"pending", "sending", "sent", "failed", "skipped"}
)

type businessMetrics struct {
	up                  bool
	duration            time.Duration
	orders, orderValues map[string]float64
	customers, payments map[string]float64
	outbox              map[string]float64
	oldestPending       float64
	reservations        float64
	overdue             float64
	outOfStock          float64
}

type metricsQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func collectBusinessMetrics(ctx context.Context, db pinger) *businessMetrics {
	queryer, ok := db.(metricsQueryer)
	if !ok {
		return nil
	}
	result := &businessMetrics{orders: map[string]float64{}, orderValues: map[string]float64{},
		customers: map[string]float64{}, payments: map[string]float64{}, outbox: map[string]float64{}}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	defer func() { result.duration = time.Since(start) }()
	rows, err := queryer.QueryContext(ctx, businessMetricsQuery)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var family, label string
		var count, extra float64
		if err := rows.Scan(&family, &label, &count, &extra); err != nil {
			return result
		}
		switch family {
		case "orders":
			result.orders[label], result.orderValues[label] = count, extra
		case "customers":
			result.customers[label] = count
		case "payments":
			result.payments[label] = count
		case "outbox":
			result.outbox[label] = count
			if extra > result.oldestPending {
				result.oldestPending = extra
			}
		case "reservations":
			result.reservations, result.overdue = count, extra
		case "stock":
			result.outOfStock = count
		}
	}
	result.up = rows.Err() == nil
	return result
}

func (b *businessMetrics) write(w promWriter) {
	if b == nil {
		return
	}
	w.family("nordwinds_business_metrics_up", "gauge", "1, wenn die Geschäftskennzahlen aus PostgreSQL gelesen werden konnten.")
	w.sample("nordwinds_business_metrics_up", "", boolValue(b.up))
	w.family("nordwinds_business_metrics_duration_seconds", "gauge", "Dauer der Abfrage der Geschäftskennzahlen.")
	w.sample("nordwinds_business_metrics_duration_seconds", "", b.duration.Seconds())
	if !b.up {
		return
	}
	w.family("nordwinds_orders", "gauge", "Bestellungen je Status.")
	for _, status := range orderStatuses {
		w.sample("nordwinds_orders", labels("status", status), b.orders[status])
	}
	w.family("nordwinds_order_value_euros", "gauge", "Summe der Bruttobestellwerte je Status in Euro.")
	for _, status := range orderStatuses {
		w.sample("nordwinds_order_value_euros", labels("status", status), b.orderValues[status])
	}
	w.family("nordwinds_customer_accounts", "gauge", "Kundenkonten (ohne Admins und Mitarbeiter) je Zustand.")
	for _, state := range customerStates {
		w.sample("nordwinds_customer_accounts", labels("state", state), b.customers[state])
	}
	w.family("nordwinds_payments", "gauge", "Simulierte Zahlungen je Zustand.")
	for _, state := range paymentStates {
		w.sample("nordwinds_payments", labels("state", state), b.payments[state])
	}
	w.family("nordwinds_notification_outbox_entries", "gauge", "E-Mail-Benachrichtigungen in der Outbox je Zustand.")
	for _, state := range outboxStates {
		w.sample("nordwinds_notification_outbox_entries", labels("state", state), b.outbox[state])
	}
	w.family("nordwinds_notification_outbox_oldest_pending_age_seconds", "gauge", "Alter der ältesten noch nicht versendeten E-Mail (0 = keine offen).")
	w.sample("nordwinds_notification_outbox_oldest_pending_age_seconds", "", b.oldestPending)
	w.family("nordwinds_inventory_reservations_active", "gauge", "Aktive Bestandsreservierungen offener Bestellungen.")
	w.sample("nordwinds_inventory_reservations_active", "", b.reservations)
	w.family("nordwinds_inventory_reservations_overdue", "gauge", "Aktive Reservierungen, die seit über 5 Minuten abgelaufen sind.")
	w.sample("nordwinds_inventory_reservations_overdue", "", b.overdue)
	w.family("nordwinds_catalog_variants_out_of_stock", "gauge", "Aktive Varianten veröffentlichter Produkte ohne Bestand.")
	w.sample("nordwinds_catalog_variants_out_of_stock", "", b.outOfStock)
}

func boolValue(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
