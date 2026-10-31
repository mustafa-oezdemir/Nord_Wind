package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

type GroupBy string

const (
	GroupTotal    GroupBy = "total"
	GroupCategory GroupBy = "category"
	GroupProduct  GroupBy = "product"
)

type Period string

const (
	PeriodWeek    Period = "week"
	PeriodMonth   Period = "month"
	PeriodQuarter Period = "quarter"
)

func ValidGroupBy(value GroupBy) bool {
	return value == GroupTotal || value == GroupCategory || value == GroupProduct
}

func ValidPeriod(value Period) bool {
	return value == PeriodWeek || value == PeriodMonth || value == PeriodQuarter
}

type Bucket struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Series struct {
	Key        string  `json:"key"`
	Label      string  `json:"label"`
	Values     []int64 `json:"values_cents"`
	TotalCents int64   `json:"total_cents"`
	Quantity   int64   `json:"quantity"`
}

type Report struct {
	GroupBy    GroupBy  `json:"group_by"`
	Period     Period   `json:"period"`
	Currency   string   `json:"currency"`
	StartDate  string   `json:"start_date"`
	EndDate    string   `json:"end_date"`
	Buckets    []Bucket `json:"buckets"`
	Series     []Series `json:"series"`
	TotalCents int64    `json:"total_cents"`
}

type Repository struct {
	db  *sql.DB
	now func() time.Time
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, now: time.Now}
}

// Sales fasst bezahlte, nicht erstattete Bestellpositionen nach Berliner Kauftag zusammen.
// Der Bestellrabatt wird centgenau auf die Positionen verteilt; Versand bleibt außen vor.
func (r *Repository) Sales(ctx context.Context, group GroupBy, period Period) (Report, error) {
	if !ValidGroupBy(group) || !ValidPeriod(period) {
		return Report{}, fmt.Errorf("ungültige Verkaufsanalyse")
	}
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return Report{}, err
	}
	start, end, buckets, granularity := reportRange(r.now().In(location), period)
	report := Report{
		GroupBy: group, Period: period, Currency: "EUR",
		StartDate: start.Format("2006-01-02"), EndDate: end.AddDate(0, 0, -1).Format("2006-01-02"),
		Buckets: buckets, Series: []Series{},
	}
	entityKey := "'total'"
	entityLabel := "'Gesamt'"
	categoryJoin := ""
	switch group {
	case GroupProduct:
		entityKey, entityLabel = "p.id::text", "p.name"
	case GroupCategory:
		entityKey, entityLabel = "COALESCE(category.id::text, 'uncategorized')", "COALESCE(category.name, 'Ohne Kategorie')"
		categoryJoin = `LEFT JOIN LATERAL (
			SELECT c.id, c.name FROM product_categories pc
			JOIN categories c ON c.id = pc.category_id
			WHERE pc.product_id = p.id
			ORDER BY c.sort_order, c.id LIMIT 1
		) category ON TRUE`
	}
	query := fmt.Sprintf(`WITH eligible_orders AS (
		SELECT o.id, MIN(e.created_at) AS paid_at, o.goods_cents, o.discount_cents
		FROM orders o
		JOIN order_status_events e ON e.order_id = o.id AND e.to_status = 'PAID'
		LEFT JOIN mock_payments payment ON payment.order_id = o.id
		WHERE o.status IN ('PAID', 'PROCESSING', 'SHIPPED', 'DELIVERED')
		  AND (payment.id IS NULL OR payment.state = 'succeeded')
		GROUP BY o.id, o.goods_cents, o.discount_cents
		HAVING MIN(e.created_at) >= $1 AND MIN(e.created_at) < $2
	), weighted AS (
		SELECT eo.id AS order_id, eo.paid_at, eo.discount_cents, oi.variant_id,
		       oi.position, oi.quantity, oi.line_total_cents,
		       CASE WHEN eo.goods_cents > 0
		         THEN floor(oi.line_total_cents::numeric * eo.discount_cents / eo.goods_cents)::bigint
		         ELSE 0 END AS base_discount
		FROM eligible_orders eo JOIN order_items oi ON oi.order_id = eo.id
	), allocated AS (
		SELECT *, line_total_cents - base_discount -
		  CASE WHEN row_number() OVER (PARTITION BY order_id ORDER BY position)
		    <= discount_cents - sum(base_discount) OVER (PARTITION BY order_id)
		  THEN 1 ELSE 0 END AS amount_cents
		FROM weighted
	)
	SELECT to_char(date_trunc($3, a.paid_at AT TIME ZONE 'Europe/Berlin'), 'YYYY-MM-DD'),
	       %s AS entity_key, %s AS entity_label,
	       sum(a.amount_cents)::bigint, sum(a.quantity)::bigint
	FROM allocated a
	JOIN product_variants variant ON variant.id = a.variant_id
	JOIN products p ON p.id = variant.product_id
	%s
	GROUP BY 1, 2, 3 ORDER BY 1, 2`, entityKey, entityLabel, categoryJoin)
	rows, err := r.db.QueryContext(ctx, query, start, end, granularity)
	if err != nil {
		return Report{}, err
	}
	defer rows.Close()
	values := make(map[string]*Series)
	bucketIndex := make(map[string]int, len(buckets))
	for index, bucket := range buckets {
		bucketIndex[bucket.Key] = index
	}
	for rows.Next() {
		var bucket, key, label string
		var amount, quantity int64
		if err := rows.Scan(&bucket, &key, &label, &amount, &quantity); err != nil {
			return Report{}, err
		}
		index, ok := bucketIndex[bucket]
		if !ok {
			continue
		}
		series := values[key]
		if series == nil {
			series = &Series{Key: key, Label: label, Values: make([]int64, len(buckets))}
			values[key] = series
		}
		series.Values[index] += amount
		series.TotalCents += amount
		series.Quantity += quantity
		report.TotalCents += amount
	}
	if err := rows.Err(); err != nil {
		return Report{}, err
	}
	for _, series := range values {
		report.Series = append(report.Series, *series)
	}
	if group == GroupTotal && len(report.Series) == 0 {
		report.Series = append(report.Series, Series{Key: "total", Label: "Gesamt", Values: make([]int64, len(buckets))})
	}
	sort.Slice(report.Series, func(i, j int) bool {
		if report.Series[i].TotalCents != report.Series[j].TotalCents {
			return report.Series[i].TotalCents > report.Series[j].TotalCents
		}
		return report.Series[i].Label < report.Series[j].Label
	})
	if len(report.Series) > 8 {
		other := Series{Key: "other", Label: "Weitere", Values: make([]int64, len(buckets))}
		for _, series := range report.Series[8:] {
			other.TotalCents += series.TotalCents
			other.Quantity += series.Quantity
			for index, amount := range series.Values {
				other.Values[index] += amount
			}
		}
		report.Series = append(report.Series[:8], other)
	}
	return report, nil
}

func reportRange(now time.Time, period Period) (time.Time, time.Time, []Bucket, string) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var start, end time.Time
	granularity := "day"
	switch period {
	case PeriodWeek:
		start, end = today.AddDate(0, 0, -6), today.AddDate(0, 0, 1)
	case PeriodMonth:
		start, end = today.AddDate(0, 0, -29), today.AddDate(0, 0, 1)
	default:
		start = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location()).AddDate(0, -2, 0)
		end = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location()).AddDate(0, 1, 0)
		granularity = "month"
	}
	buckets := make([]Bucket, 0, 30)
	for at := start; at.Before(end); {
		label := at.Format("02.01.")
		if granularity == "month" {
			label = [...]string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}[at.Month()-1] + " " + at.Format("2006")
		}
		buckets = append(buckets, Bucket{Key: at.Format("2006-01-02"), Label: label})
		if granularity == "month" {
			at = at.AddDate(0, 1, 0)
		} else {
			at = at.AddDate(0, 0, 1)
		}
	}
	return start, end, buckets, granularity
}
