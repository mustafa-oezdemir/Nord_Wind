package catalog

import (
	"context"
	"database/sql"
	"time"
)

type AdminRepository struct{ db *sql.DB }

func NewAdminRepository(db *sql.DB) *AdminRepository { return &AdminRepository{db: db} }

type AuditEvent struct {
	ID              int64     `json:"id"`
	ActorCustomerID string    `json:"actor_customer_id"`
	EntityType      string    `json:"entity_type"`
	EntityID        string    `json:"entity_id"`
	RelatedID       *string   `json:"related_id"`
	Action          string    `json:"action"`
	CreatedAt       time.Time `json:"created_at"`
}

func (a *AdminRepository) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, parent_id, name, slug, description, status,
		sort_order, created_at, updated_at FROM categories ORDER BY sort_order, name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Category{}
	for rows.Next() {
		item, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (a *AdminRepository) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, name, slug, description, status,
		created_at, updated_at FROM products ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Product{}
	for rows.Next() {
		item, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (a *AdminRepository) ListVariants(ctx context.Context, productID string) ([]ProductVariant, error) {
	return NewRepository(a.db).ListProductVariants(ctx, productID)
}

func (a *AdminRepository) GetCategory(ctx context.Context, id string) (Category, error) {
	return NewRepository(a.db).GetCategory(ctx, id)
}

func (a *AdminRepository) GetProduct(ctx context.Context, id string) (Product, error) {
	return NewRepository(a.db).GetProduct(ctx, id)
}

func (a *AdminRepository) GetVariant(ctx context.Context, id string) (ProductVariant, error) {
	return NewRepository(a.db).GetVariant(ctx, id)
}

func (a *AdminRepository) ListProductCategories(ctx context.Context, productID string) ([]Category, error) {
	return NewRepository(a.db).ListProductCategories(ctx, productID)
}

func (a *AdminRepository) CreateCategory(ctx context.Context, actorID string, input Category) (Category, error) {
	var result Category
	err := a.mutate(ctx, actorID, "category", "create", func(r *Repository) (string, error) {
		var err error
		result, err = r.CreateCategory(ctx, input)
		return result.ID, err
	})
	return result, err
}

func (a *AdminRepository) UpdateCategory(ctx context.Context, actorID string, input Category) (Category, error) {
	var result Category
	err := a.mutate(ctx, actorID, "category", "update", func(r *Repository) (string, error) {
		var err error
		result, err = r.UpdateCategory(ctx, input)
		return result.ID, err
	})
	return result, err
}

func (a *AdminRepository) DeleteCategory(ctx context.Context, actorID, id string) error {
	return a.mutate(ctx, actorID, "category", "delete", func(r *Repository) (string, error) {
		return id, r.DeleteCategory(ctx, id)
	})
}

func (a *AdminRepository) CreateProduct(ctx context.Context, actorID string, input Product) (Product, error) {
	var result Product
	err := a.mutate(ctx, actorID, "product", "create", func(r *Repository) (string, error) {
		var err error
		result, err = r.CreateProduct(ctx, input)
		return result.ID, err
	})
	return result, err
}

func (a *AdminRepository) UpdateProduct(ctx context.Context, actorID string, input Product) (Product, error) {
	var result Product
	err := a.mutate(ctx, actorID, "product", "update", func(r *Repository) (string, error) {
		var err error
		result, err = r.UpdateProduct(ctx, input)
		return result.ID, err
	})
	return result, err
}

func (a *AdminRepository) DeleteProduct(ctx context.Context, actorID, id string) error {
	return a.mutate(ctx, actorID, "product", "delete", func(r *Repository) (string, error) {
		return id, r.DeleteProduct(ctx, id)
	})
}

func (a *AdminRepository) CreateVariant(ctx context.Context, actorID string, input ProductVariant) (ProductVariant, error) {
	var result ProductVariant
	err := a.mutate(ctx, actorID, "variant", "create", func(r *Repository) (string, error) {
		var err error
		result, err = r.CreateVariant(ctx, input)
		return result.ID, err
	})
	return result, err
}

func (a *AdminRepository) UpdateVariant(ctx context.Context, actorID string, input ProductVariant) (ProductVariant, error) {
	var result ProductVariant
	err := a.mutate(ctx, actorID, "variant", "update", func(r *Repository) (string, error) {
		var err error
		result, err = r.UpdateVariant(ctx, input)
		return result.ID, err
	})
	return result, err
}

func (a *AdminRepository) DeleteVariant(ctx context.Context, actorID, id string) error {
	return a.mutate(ctx, actorID, "variant", "delete", func(r *Repository) (string, error) {
		return id, r.DeleteVariant(ctx, id)
	})
}

func (a *AdminRepository) AttachCategory(ctx context.Context, actorID, productID, categoryID string) error {
	return a.mutateWithRelated(ctx, actorID, "product_category", "attach", categoryID, func(r *Repository) (string, error) {
		return productID, r.AddProductCategory(ctx, productID, categoryID)
	})
}

func (a *AdminRepository) DetachCategory(ctx context.Context, actorID, productID, categoryID string) error {
	return a.mutateWithRelated(ctx, actorID, "product_category", "detach", categoryID, func(r *Repository) (string, error) {
		return productID, r.RemoveProductCategory(ctx, productID, categoryID)
	})
}

func (a *AdminRepository) ListAudit(ctx context.Context) ([]AuditEvent, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, actor_customer_id, entity_type, entity_id, related_id, action, created_at
		FROM admin_audit_events ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AuditEvent{}
	for rows.Next() {
		var event AuditEvent
		var related sql.NullString
		if err := rows.Scan(&event.ID, &event.ActorCustomerID, &event.EntityType,
			&event.EntityID, &related, &event.Action, &event.CreatedAt); err != nil {
			return nil, err
		}
		if related.Valid {
			event.RelatedID = &related.String
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (a *AdminRepository) mutate(ctx context.Context, actorID, entityType, action string, change func(*Repository) (string, error)) error {
	return a.mutateWithRelated(ctx, actorID, entityType, action, "", change)
}

func (a *AdminRepository) mutateWithRelated(ctx context.Context, actorID, entityType, action, relatedID string, change func(*Repository) (string, error)) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	entityID, err := change(NewRepository(tx))
	if err != nil {
		return err
	}
	var related sql.NullString
	if relatedID != "" {
		related = sql.NullString{String: relatedID, Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO admin_audit_events
		(actor_customer_id, entity_type, entity_id, related_id, action) VALUES ($1, $2, $3, $4, $5)`,
		actorID, entityType, entityID, related, action); err != nil {
		return err
	}
	return tx.Commit()
}
