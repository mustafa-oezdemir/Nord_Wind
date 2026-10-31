package analytics

import (
	"context"
	"database/sql"
	"errors"
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
	// PeriodCustom nutzt From und To (jeweils einschließlich, Kalendertage in Berlin).
	PeriodCustom Period = "custom"
)

const (
	// MaxCustomDays begrenzt einen eigenen Zeitraum auf gut zwei Jahre.
	MaxCustomDays = 731
	// dailyLimit: Bis zu dieser Länge zeigt ein eigener Zeitraum Tage, darüber Monate.
	dailyLimit = 62
)

var (
	// ErrInvalidQuery kennzeichnet ungültige Parameter (HTTP 400).
	ErrInvalidQuery = errors.New("ungültige Verkaufsanalyse")
	// ErrNotFound kennzeichnet eine unbekannte Kategorie oder ein unbekanntes Produkt (HTTP 404).
	ErrNotFound = errors.New("Kategorie oder Produkt nicht gefunden")
)

func ValidGroupBy(value GroupBy) bool {
	return value == GroupTotal || value == GroupCategory || value == GroupProduct
}

func ValidPeriod(value Period) bool {
	return value == PeriodWeek || value == PeriodMonth || value == PeriodQuarter || value == PeriodCustom
}

// Query beschreibt eine Auswertung. CategoryID schränkt auf die Kategorie und alle
// Unterkategorien ein, ProductID auf ein einzelnes Produkt (nur für Gesamt und Produkte).
type Query struct {
	GroupBy    GroupBy
	Period     Period
	From, To   string
	CategoryID string
	ProductID  string
}

type Bucket struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Series struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// CategoryKey ist bei Produkten die primäre Kategorie, bei Kategorien die Kategorie selbst.
	CategoryKey   string  `json:"category_key"`
	CategoryLabel string  `json:"category_label"`
	ParentKey     string  `json:"parent_key,omitempty"`
	Depth         int     `json:"depth"`
	Values        []int64 `json:"values_cents"`
	TotalCents    int64   `json:"total_cents"`
	Quantity      int64   `json:"quantity"`
}

type Report struct {
	GroupBy     GroupBy  `json:"group_by"`
	Period      Period   `json:"period"`
	Granularity string   `json:"granularity"`
	CategoryID  string   `json:"category_id,omitempty"`
	ProductID   string   `json:"product_id,omitempty"`
	Currency    string   `json:"currency"`
	StartDate   string   `json:"start_date"`
	EndDate     string   `json:"end_date"`
	Buckets     []Bucket `json:"buckets"`
	Series      []Series `json:"series"`
	TotalCents  int64    `json:"total_cents"`
}

// CatalogCategory und CatalogProduct speisen die Auswahllisten der Analyse.
type CatalogCategory struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
	Status   string `json:"status"`
	Depth    int    `json:"depth"`
}

type CatalogProduct struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	CategoryID string `json:"category_id,omitempty"`
}

type Catalog struct {
	Categories []CatalogCategory `json:"categories"`
	Products   []CatalogProduct  `json:"products"`
}

const uncategorized = "uncategorized"

type Repository struct {
	db  *sql.DB
	now func() time.Time
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, now: time.Now}
}

// Catalog liefert alle Kategorien in Baumreihenfolge (sort_order, Name) und alle Produkte
// mit ihrer primären Kategorie, gruppiert nach dieser Reihenfolge.
func (r *Repository) Catalog(ctx context.Context) (Catalog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id::text, name, COALESCE(parent_id::text, ''), status, sort_order FROM categories`)
	if err != nil {
		return Catalog{}, err
	}
	defer rows.Close()
	type node struct {
		category CatalogCategory
		sort     int
	}
	children := map[string][]node{}
	known := map[string]bool{}
	for rows.Next() {
		var n node
		if err := rows.Scan(&n.category.ID, &n.category.Name, &n.category.ParentID, &n.category.Status, &n.sort); err != nil {
			return Catalog{}, err
		}
		children[n.category.ParentID] = append(children[n.category.ParentID], n)
		known[n.category.ID] = true
	}
	if err := rows.Err(); err != nil {
		return Catalog{}, err
	}
	catalog := Catalog{Categories: []CatalogCategory{}, Products: []CatalogProduct{}}
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		list := children[parent]
		sort.Slice(list, func(i, j int) bool {
			if list[i].sort != list[j].sort {
				return list[i].sort < list[j].sort
			}
			if list[i].category.Name != list[j].category.Name {
				return list[i].category.Name < list[j].category.Name
			}
			return list[i].category.ID < list[j].category.ID
		})
		for _, n := range list {
			n.category.Depth = depth
			catalog.Categories = append(catalog.Categories, n.category)
			walk(n.category.ID, depth+1)
		}
	}
	walk("", 0)

	products, err := r.db.QueryContext(ctx, `SELECT p.id::text, p.name, p.status, COALESCE(primary_category.id::text, '')
		FROM products p
		LEFT JOIN LATERAL (
			SELECT c.id FROM product_categories pc JOIN categories c ON c.id = pc.category_id
			WHERE pc.product_id = p.id ORDER BY c.sort_order, c.id LIMIT 1
		) primary_category ON TRUE`)
	if err != nil {
		return Catalog{}, err
	}
	defer products.Close()
	for products.Next() {
		var product CatalogProduct
		if err := products.Scan(&product.ID, &product.Name, &product.Status, &product.CategoryID); err != nil {
			return Catalog{}, err
		}
		catalog.Products = append(catalog.Products, product)
	}
	if err := products.Err(); err != nil {
		return Catalog{}, err
	}
	order := map[string]int{"": len(catalog.Categories)}
	for index, category := range catalog.Categories {
		order[category.ID] = index
	}
	sort.SliceStable(catalog.Products, func(i, j int) bool {
		a, b := catalog.Products[i], catalog.Products[j]
		if order[a.CategoryID] != order[b.CategoryID] {
			return order[a.CategoryID] < order[b.CategoryID]
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return catalog, nil
}

func (r *Repository) Sales(ctx context.Context, query Query) (Report, error) {
	if !ValidGroupBy(query.GroupBy) || !ValidPeriod(query.Period) {
		return Report{}, ErrInvalidQuery
	}
	if query.ProductID != "" && query.GroupBy == GroupCategory {
		return Report{}, ErrInvalidQuery
	}
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return Report{}, err
	}
	start, end, buckets, granularity, err := reportRange(r.now().In(location), query)
	if err != nil {
		return Report{}, err
	}
	catalog, err := r.Catalog(ctx)
	if err != nil {
		return Report{}, err
	}

	// Umfang: gewählte Kategorie samt Unterkategorien, gewähltes Produkt oder alles.
	categoryIndex := map[string]CatalogCategory{}
	for _, category := range catalog.Categories {
		categoryIndex[category.ID] = category
	}
	inCategoryScope := func(string) bool { return true }
	if query.CategoryID != "" {
		if _, ok := categoryIndex[query.CategoryID]; !ok {
			return Report{}, ErrNotFound
		}
		scope := descendants(catalog.Categories, query.CategoryID)
		inCategoryScope = func(id string) bool { return scope[id] }
	}
	productCategory := map[string]string{}
	products := []CatalogProduct{}
	for _, product := range catalog.Products {
		productCategory[product.ID] = product.CategoryID
		if query.ProductID != "" && product.ID != query.ProductID {
			continue
		}
		if query.CategoryID != "" && !inCategoryScope(product.CategoryID) {
			continue
		}
		products = append(products, product)
	}
	if query.ProductID != "" && len(products) == 0 {
		if _, exists := productCategory[query.ProductID]; exists {
			return Report{}, ErrInvalidQuery // Produkt gehört nicht zur gewählten Kategorie
		}
		return Report{}, ErrNotFound
	}
	includedProducts := map[string]bool{}
	for _, product := range products {
		includedProducts[product.ID] = true
	}

	report := Report{
		GroupBy: query.GroupBy, Period: query.Period, Granularity: granularity,
		CategoryID: query.CategoryID, ProductID: query.ProductID, Currency: "EUR",
		StartDate: start.Format("2006-01-02"), EndDate: end.AddDate(0, 0, -1).Format("2006-01-02"),
		Buckets: buckets, Series: []Series{},
	}
	series := map[string]*Series{}
	var order []string
	add := func(s Series) {
		s.Values = make([]int64, len(buckets))
		series[s.Key] = &s
		order = append(order, s.Key)
	}
	switch query.GroupBy {
	case GroupTotal:
		add(Series{Key: "total", Label: "Gesamt"})
	case GroupCategory:
		for _, category := range catalog.Categories {
			if inCategoryScope(category.ID) {
				add(Series{Key: category.ID, Label: category.Name, CategoryKey: category.ID, CategoryLabel: category.Name,
					ParentKey: category.ParentID, Depth: category.Depth})
			}
		}
		if query.CategoryID == "" {
			for _, product := range catalog.Products {
				if product.CategoryID == "" {
					add(Series{Key: uncategorized, Label: "Ohne Kategorie", CategoryKey: uncategorized, CategoryLabel: "Ohne Kategorie"})
					break
				}
			}
		}
	case GroupProduct:
		for _, product := range products {
			categoryKey, categoryLabel := uncategorized, "Ohne Kategorie"
			if category, ok := categoryIndex[product.CategoryID]; ok {
				categoryKey, categoryLabel = category.ID, category.Name
			}
			add(Series{Key: product.ID, Label: product.Name, CategoryKey: categoryKey, CategoryLabel: categoryLabel})
		}
	}

	rows, err := r.db.QueryContext(ctx, `WITH eligible_orders AS (
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
	       variant.product_id::text, sum(a.amount_cents)::bigint, sum(a.quantity)::bigint
	FROM allocated a
	JOIN product_variants variant ON variant.id = a.variant_id
	GROUP BY 1, 2 ORDER BY 1, 2`, start, end, granularity)
	if err != nil {
		return Report{}, err
	}
	defer rows.Close()
	bucketIndex := make(map[string]int, len(buckets))
	for index, bucket := range buckets {
		bucketIndex[bucket.Key] = index
	}
	for rows.Next() {
		var bucket, productID string
		var amount, quantity int64
		if err := rows.Scan(&bucket, &productID, &amount, &quantity); err != nil {
			return Report{}, err
		}
		index, ok := bucketIndex[bucket]
		if !ok || !includedProducts[productID] {
			continue
		}
		key := "total"
		switch query.GroupBy {
		case GroupCategory:
			key = productCategory[productID]
			if key == "" {
				key = uncategorized
			}
		case GroupProduct:
			key = productID
		}
		target := series[key]
		if target == nil {
			continue
		}
		target.Values[index] += amount
		target.TotalCents += amount
		target.Quantity += quantity
		report.TotalCents += amount
	}
	if err := rows.Err(); err != nil {
		return Report{}, err
	}
	for _, key := range order {
		report.Series = append(report.Series, *series[key])
	}
	if query.GroupBy == GroupProduct {
		// Innerhalb einer Kategorie stehen umsatzstarke Produkte zuerst; die Kategorien
		// selbst behalten die Reihenfolge des Katalogbaums.
		rank := map[string]int{}
		for index, s := range report.Series {
			if _, seen := rank[s.CategoryKey]; !seen {
				rank[s.CategoryKey] = index
			}
		}
		sort.SliceStable(report.Series, func(i, j int) bool {
			a, b := report.Series[i], report.Series[j]
			if a.CategoryKey != b.CategoryKey {
				return rank[a.CategoryKey] < rank[b.CategoryKey]
			}
			return a.TotalCents > b.TotalCents
		})
	}
	return report, nil
}

func descendants(categories []CatalogCategory, root string) map[string]bool {
	scope := map[string]bool{root: true}
	for changed := true; changed; {
		changed = false
		for _, category := range categories {
			if !scope[category.ID] && scope[category.ParentID] {
				scope[category.ID] = true
				changed = true
			}
		}
	}
	return scope
}

var monthNames = [...]string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}

func reportRange(now time.Time, query Query) (time.Time, time.Time, []Bucket, string, error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var start, end time.Time
	granularity := "day"
	switch query.Period {
	case PeriodWeek:
		start, end = today.AddDate(0, 0, -6), today.AddDate(0, 0, 1)
	case PeriodMonth:
		start, end = today.AddDate(0, 0, -29), today.AddDate(0, 0, 1)
	case PeriodQuarter:
		start = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location()).AddDate(0, -2, 0)
		end = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location()).AddDate(0, 1, 0)
		granularity = "month"
	case PeriodCustom:
		from, fromErr := time.ParseInLocation("2006-01-02", query.From, now.Location())
		to, toErr := time.ParseInLocation("2006-01-02", query.To, now.Location())
		if fromErr != nil || toErr != nil || to.Before(from) || to.After(today) {
			return time.Time{}, time.Time{}, nil, "", ErrInvalidQuery
		}
		days := int(to.Sub(from).Hours()/24+0.5) + 1
		if days > MaxCustomDays {
			return time.Time{}, time.Time{}, nil, "", ErrInvalidQuery
		}
		start, end = from, to.AddDate(0, 0, 1)
		if days > dailyLimit {
			granularity = "month"
		}
	default:
		return time.Time{}, time.Time{}, nil, "", ErrInvalidQuery
	}
	if query.Period != PeriodCustom && (query.From != "" || query.To != "") {
		return time.Time{}, time.Time{}, nil, "", ErrInvalidQuery
	}
	buckets := make([]Bucket, 0, 31)
	at := start
	if granularity == "month" {
		at = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	}
	for at.Before(end) {
		label := at.Format("02.01.")
		if granularity == "month" {
			label = monthNames[at.Month()-1] + " " + at.Format("2006")
		} else if start.Year() != end.AddDate(0, 0, -1).Year() {
			label = at.Format("02.01.06")
		}
		buckets = append(buckets, Bucket{Key: at.Format("2006-01-02"), Label: label})
		if granularity == "month" {
			at = at.AddDate(0, 1, 0)
		} else {
			at = at.AddDate(0, 0, 1)
		}
	}
	return start, end, buckets, granularity, nil
}
