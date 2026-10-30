package catalog

import (
	"context"
	"database/sql"
	"errors"
)

const MaxProductImages = 6

var ErrImageLimit = errors.New("Produktbildlimit erreicht")

type ProductImage struct {
	ID     string `json:"id"`
	Src    string `json:"src"`
	Alt    string `json:"alt"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func imageURL(id string) string { return "/api/v1/media/products/" + id }

func (r *Repository) ListProductImages(ctx context.Context, productID string) ([]ProductImage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, alt_text, width, height FROM product_images
		WHERE product_id = $1 ORDER BY created_at, id`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	images := []ProductImage{}
	for rows.Next() {
		var image ProductImage
		if err := rows.Scan(&image.ID, &image.Alt, &image.Width, &image.Height); err != nil {
			return nil, err
		}
		image.Src = imageURL(image.ID)
		images = append(images, image)
	}
	return images, rows.Err()
}

func (r *Repository) GetPublicImage(ctx context.Context, id string) ([]byte, error) {
	var data []byte
	err := r.db.QueryRowContext(ctx, `SELECT i.image_bytes FROM product_images i
		JOIN products p ON p.id = i.product_id
		WHERE i.id = $1 AND p.status IN ('published', 'archived')`, id).Scan(&data)
	return data, notFound(err)
}

func (a *AdminRepository) ListImages(ctx context.Context, productID string) ([]ProductImage, error) {
	return NewRepository(a.db).ListProductImages(ctx, productID)
}

func (a *AdminRepository) CreateImage(ctx context.Context, actorID, productID, alt string, width, height int, data []byte) (ProductImage, error) {
	var image ProductImage
	err := a.mutate(ctx, actorID, "image", "create", func(r *Repository) (string, error) {
		// Parent row lock serialisiert parallele Uploads für dasselbe Produkt.
		var lockedID string
		if err := r.db.QueryRowContext(ctx, `SELECT id FROM products WHERE id = $1 FOR UPDATE`, productID).Scan(&lockedID); err != nil {
			return "", notFound(err)
		}
		var count int
		if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM product_images WHERE product_id = $1`, productID).Scan(&count); err != nil {
			return "", err
		}
		if count >= MaxProductImages {
			return "", ErrImageLimit
		}
		image = ProductImage{Alt: alt, Width: width, Height: height}
		err := r.db.QueryRowContext(ctx, `INSERT INTO product_images
			(product_id, alt_text, width, height, image_bytes)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`, productID, alt, width, height, data).Scan(&image.ID)
		image.Src = imageURL(image.ID)
		return image.ID, err
	})
	return image, err
}

func (a *AdminRepository) DeleteImage(ctx context.Context, actorID, id string) error {
	return a.mutate(ctx, actorID, "image", "delete", func(r *Repository) (string, error) {
		var deletedID string
		err := r.db.QueryRowContext(ctx, `DELETE FROM product_images WHERE id = $1 RETURNING id`, id).Scan(&deletedID)
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return deletedID, err
	})
}
