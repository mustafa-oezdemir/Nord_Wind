package catalog

import (
	"context"
	"database/sql"
	"errors"
)

// ErrNotFound bezeichnet einen fehlenden Katalogdatensatz.
var ErrNotFound = errors.New("Katalogdatensatz nicht gefunden")

// Queryer erlaubt dieselben Abfragen innerhalb einer vom Aufrufer verwalteten Transaktion.
type Queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Repository struct {
	db Queryer
}

func NewRepository(db Queryer) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateCategory(ctx context.Context, category Category) (Category, error) {
	if category.Status == "" {
		category.Status = CategoryActive
	}
	err := r.db.QueryRowContext(ctx, `INSERT INTO categories
		(parent_id, name, slug, description, status, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`, category.ParentID, category.Name, category.Slug,
		category.Description, category.Status, category.SortOrder,
	).Scan(&category.ID, &category.CreatedAt, &category.UpdatedAt)
	return category, err
}

func (r *Repository) GetCategory(ctx context.Context, id string) (Category, error) {
	return scanCategory(r.db.QueryRowContext(ctx, `SELECT id, parent_id, name, slug, description,
		status, sort_order, created_at, updated_at FROM categories WHERE id = $1`, id))
}

func (r *Repository) UpdateCategory(ctx context.Context, category Category) (Category, error) {
	err := r.db.QueryRowContext(ctx, `UPDATE categories SET parent_id = $2, name = $3, slug = $4,
		description = $5, status = $6, sort_order = $7, updated_at = now()
		WHERE id = $1 RETURNING updated_at`, category.ID, category.ParentID, category.Name,
		category.Slug, category.Description, category.Status, category.SortOrder,
	).Scan(&category.UpdatedAt)
	return category, notFound(err)
}

func (r *Repository) DeleteCategory(ctx context.Context, id string) error {
	return deleteByID(ctx, r.db, `DELETE FROM categories WHERE id = $1`, id)
}

func (r *Repository) CreateProduct(ctx context.Context, product Product) (Product, error) {
	if product.Status == "" {
		product.Status = ProductDraft
	}
	err := r.db.QueryRowContext(ctx, `INSERT INTO products (name, slug, description, status)
		VALUES ($1, $2, $3, $4) RETURNING id, created_at, updated_at`, product.Name,
		product.Slug, product.Description, product.Status,
	).Scan(&product.ID, &product.CreatedAt, &product.UpdatedAt)
	return product, err
}

func (r *Repository) GetProduct(ctx context.Context, id string) (Product, error) {
	return scanProduct(r.db.QueryRowContext(ctx, `SELECT id, name, slug, description, status,
		created_at, updated_at FROM products WHERE id = $1`, id))
}

func (r *Repository) UpdateProduct(ctx context.Context, product Product) (Product, error) {
	err := r.db.QueryRowContext(ctx, `UPDATE products SET name = $2, slug = $3, description = $4,
		status = $5, updated_at = now() WHERE id = $1 RETURNING updated_at`, product.ID,
		product.Name, product.Slug, product.Description, product.Status,
	).Scan(&product.UpdatedAt)
	return product, notFound(err)
}

func (r *Repository) DeleteProduct(ctx context.Context, id string) error {
	return deleteByID(ctx, r.db, `DELETE FROM products WHERE id = $1`, id)
}

func (r *Repository) AddProductCategory(ctx context.Context, productID, categoryID string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO product_categories (product_id, category_id)
		VALUES ($1, $2)`, productID, categoryID)
	return err
}

func (r *Repository) RemoveProductCategory(ctx context.Context, productID, categoryID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM product_categories
		WHERE product_id = $1 AND category_id = $2`, productID, categoryID)
	return affected(result, err)
}

func (r *Repository) ListProductCategories(ctx context.Context, productID string) ([]Category, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT c.id, c.parent_id, c.name, c.slug, c.description,
		c.status, c.sort_order, c.created_at, c.updated_at
		FROM categories c JOIN product_categories pc ON pc.category_id = c.id
		WHERE pc.product_id = $1 ORDER BY c.sort_order, c.id`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var categories []Category
	for rows.Next() {
		category, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

func (r *Repository) CreateVariant(ctx context.Context, variant ProductVariant) (ProductVariant, error) {
	if variant.Currency == "" {
		variant.Currency = "EUR"
	}
	err := r.db.QueryRowContext(ctx, `INSERT INTO product_variants
		(product_id, sku, label, price_cents, currency, stock_quantity, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`, variant.ProductID, variant.SKU, variant.Label,
		variant.PriceCents, variant.Currency, variant.StockQuantity, variant.IsActive,
	).Scan(&variant.ID, &variant.CreatedAt, &variant.UpdatedAt)
	return variant, err
}

func (r *Repository) GetVariant(ctx context.Context, id string) (ProductVariant, error) {
	return scanVariant(r.db.QueryRowContext(ctx, `SELECT id, product_id, sku, label, price_cents,
		currency, stock_quantity, is_active, created_at, updated_at
		FROM product_variants WHERE id = $1`, id))
}

func (r *Repository) UpdateVariant(ctx context.Context, variant ProductVariant) (ProductVariant, error) {
	err := r.db.QueryRowContext(ctx, `UPDATE product_variants SET sku = $2, label = $3,
		price_cents = $4, currency = $5, stock_quantity = $6, is_active = $7,
		updated_at = now() WHERE id = $1 RETURNING updated_at`, variant.ID, variant.SKU,
		variant.Label, variant.PriceCents, variant.Currency, variant.StockQuantity, variant.IsActive,
	).Scan(&variant.UpdatedAt)
	return variant, notFound(err)
}

func (r *Repository) DeleteVariant(ctx context.Context, id string) error {
	return deleteByID(ctx, r.db, `DELETE FROM product_variants WHERE id = $1`, id)
}

func (r *Repository) ListProductVariants(ctx context.Context, productID string) ([]ProductVariant, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, product_id, sku, label, price_cents,
		currency, stock_quantity, is_active, created_at, updated_at
		FROM product_variants WHERE product_id = $1 ORDER BY id`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var variants []ProductVariant
	for rows.Next() {
		variant, err := scanVariant(rows)
		if err != nil {
			return nil, err
		}
		variants = append(variants, variant)
	}
	return variants, rows.Err()
}

type scanner interface {
	Scan(...any) error
}

func scanCategory(row scanner) (Category, error) {
	var category Category
	var parent sql.NullString
	err := row.Scan(&category.ID, &parent, &category.Name, &category.Slug,
		&category.Description, &category.Status, &category.SortOrder,
		&category.CreatedAt, &category.UpdatedAt)
	if parent.Valid {
		category.ParentID = &parent.String
	}
	return category, notFound(err)
}

func scanProduct(row scanner) (Product, error) {
	var product Product
	err := row.Scan(&product.ID, &product.Name, &product.Slug, &product.Description,
		&product.Status, &product.CreatedAt, &product.UpdatedAt)
	return product, notFound(err)
}

func scanVariant(row scanner) (ProductVariant, error) {
	var variant ProductVariant
	err := row.Scan(&variant.ID, &variant.ProductID, &variant.SKU, &variant.Label,
		&variant.PriceCents, &variant.Currency, &variant.StockQuantity, &variant.IsActive,
		&variant.CreatedAt, &variant.UpdatedAt)
	return variant, notFound(err)
}

func deleteByID(ctx context.Context, db Queryer, query, id string) error {
	result, err := db.ExecContext(ctx, query, id)
	return affected(result, err)
}

func affected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
