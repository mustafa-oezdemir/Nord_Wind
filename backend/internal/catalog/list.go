package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidListOptions = errors.New("ungültige Katalogfilter")

// ListOptions enthält bereits begrenzte Seitengröße und einen sicheren Offset.
type ListOptions struct {
	Category string
	Query    string
	Sort     string
	Limit    int
	Offset   int
}

type Page[T any] struct {
	Items []T
	Total int64
}

// ProductSummary enthält nur öffentlich darstellbare, serverseitig berechnete Werte.
type ProductSummary struct {
	ID           string
	Slug         string
	Name         string
	Description  string
	PriceCents   int64
	Currency     string
	Available    bool
	VariantCount int64
}

func (r *Repository) ListProducts(ctx context.Context, options ListOptions) (Page[ProductSummary], error) {
	result := Page[ProductSummary]{Items: []ProductSummary{}}
	orderBy, ok := productOrder(options.Sort)
	if !ok || !validPage(options) {
		return result, ErrInvalidListOptions
	}
	args := make([]any, 0, 4)
	base := ` FROM products p
		JOIN LATERAL (
			SELECT min(v.price_cents) AS price_cents,
				bool_or(v.stock_quantity > 0) AS available, count(*) AS variant_count
			FROM product_variants v WHERE v.product_id = p.id AND v.is_active
		) offer ON offer.variant_count > 0
		WHERE p.status = 'published'`
	if options.Category != "" {
		categoryID, err := r.activeCategoryID(ctx, options.Category)
		if err != nil {
			return result, err
		}
		args = append(args, categoryID)
		base += fmt.Sprintf(` AND EXISTS (
			WITH RECURSIVE descendants(id) AS (
				SELECT id FROM categories WHERE id = $%d
				UNION ALL
				SELECT c.id FROM categories c JOIN descendants d ON c.parent_id = d.id
				WHERE c.status = 'active'
			)
			SELECT 1 FROM product_categories pc JOIN descendants d ON d.id = pc.category_id
			WHERE pc.product_id = p.id
		)`, len(args))
	}
	if options.Query != "" {
		args = append(args, likePattern(options.Query))
		placeholder := fmt.Sprintf("$%d", len(args))
		base += ` AND (p.name ILIKE ` + placeholder + ` ESCAPE '!'
			OR p.description ILIKE ` + placeholder + ` ESCAPE '!'
			OR EXISTS (SELECT 1 FROM product_variants search_variant
				WHERE search_variant.product_id = p.id AND search_variant.is_active
				AND search_variant.sku ILIKE ` + placeholder + ` ESCAPE '!'))`
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*)`+base, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	limitPlaceholder := fmt.Sprintf("$%d", len(args)+1)
	offsetPlaceholder := fmt.Sprintf("$%d", len(args)+2)
	args = append(args, options.Limit, options.Offset)
	rows, err := r.db.QueryContext(ctx, `SELECT p.id, p.slug, p.name, p.description,
		offer.price_cents, 'EUR', offer.available, offer.variant_count`+base+
		` ORDER BY `+orderBy+` LIMIT `+limitPlaceholder+` OFFSET `+offsetPlaceholder, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var product ProductSummary
		if err := rows.Scan(&product.ID, &product.Slug, &product.Name, &product.Description,
			&product.PriceCents, &product.Currency, &product.Available, &product.VariantCount); err != nil {
			return result, err
		}
		result.Items = append(result.Items, product)
	}
	return result, rows.Err()
}

func (r *Repository) ListCategories(ctx context.Context, options ListOptions) (Page[Category], error) {
	result := Page[Category]{Items: []Category{}}
	orderBy, ok := categoryOrder(options.Sort)
	if !ok || !validPage(options) {
		return result, ErrInvalidListOptions
	}
	args := make([]any, 0, 4)
	base := ` FROM categories c WHERE c.status = 'active'`
	if options.Category != "" {
		categoryID, err := r.activeCategoryID(ctx, options.Category)
		if err != nil {
			return result, err
		}
		args = append(args, categoryID)
		base += fmt.Sprintf(` AND c.parent_id = $%d`, len(args))
	}
	if options.Query != "" {
		args = append(args, likePattern(options.Query))
		placeholder := fmt.Sprintf("$%d", len(args))
		base += ` AND (c.name ILIKE ` + placeholder + ` ESCAPE '!'
			OR c.description ILIKE ` + placeholder + ` ESCAPE '!')`
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*)`+base, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	limitPlaceholder := fmt.Sprintf("$%d", len(args)+1)
	offsetPlaceholder := fmt.Sprintf("$%d", len(args)+2)
	args = append(args, options.Limit, options.Offset)
	rows, err := r.db.QueryContext(ctx, `SELECT c.id, c.parent_id, c.name, c.slug,
		c.description, c.status, c.sort_order, c.created_at, c.updated_at`+base+
		` ORDER BY `+orderBy+` LIMIT `+limitPlaceholder+` OFFSET `+offsetPlaceholder, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		category, err := scanCategory(rows)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, category)
	}
	return result, rows.Err()
}

func (r *Repository) activeCategoryID(ctx context.Context, slug string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM categories
		WHERE lower(slug) = lower($1) AND status = 'active'`, slug).Scan(&id)
	return id, notFound(err)
}

func validPage(options ListOptions) bool {
	return options.Limit >= 1 && options.Limit <= 100 && options.Offset >= 0 && options.Offset <= 999900
}

func productOrder(sort string) (string, bool) {
	switch sort {
	case "", "newest":
		return "p.created_at DESC, p.id DESC", true
	case "name_asc":
		return "p.name ASC, p.id ASC", true
	case "price_asc":
		return "offer.price_cents ASC, p.id ASC", true
	case "price_desc":
		return "offer.price_cents DESC, p.id ASC", true
	default:
		return "", false
	}
}

func categoryOrder(sort string) (string, bool) {
	switch sort {
	case "", "sort_order":
		return "c.sort_order ASC, c.name ASC, c.id ASC", true
	case "name_asc":
		return "c.name ASC, c.id ASC", true
	case "name_desc":
		return "c.name DESC, c.id ASC", true
	default:
		return "", false
	}
}

func likePattern(query string) string {
	replacer := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return "%" + replacer.Replace(query) + "%"
}
