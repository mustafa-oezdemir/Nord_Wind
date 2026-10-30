package server

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
)

const maxProductSlugRunes = 120

// registerProductDetailRoute liefert ein öffentliches Produkt mit aktiven Varianten.
// Preise und Bestandsstatus stammen ausschließlich aus der Datenbank.
func registerProductDetailRoute(router *gin.Engine, repository *catalog.Repository) {
	router.GET("/api/v1/products/:slug", func(c *gin.Context) {
		slug, ok := productSlug(c)
		if !ok {
			writeCatalogProblem(c, http.StatusBadRequest)
			return
		}
		detail, err := repository.GetPublicProduct(c.Request.Context(), slug)
		if err != nil {
			writeCatalogError(c, err)
			return
		}
		c.JSON(http.StatusOK, newProductDetailResponse(detail))
	})
}

// productSlug weist Query-Parameter und unplausible Slugs ab, bevor die Datenbank gefragt wird.
func productSlug(c *gin.Context) (string, bool) {
	if c.Request.URL.RawQuery != "" {
		return "", false
	}
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" || !utf8.ValidString(slug) || strings.ContainsRune(slug, 0) ||
		utf8.RuneCountInString(slug) > maxProductSlugRunes {
		return "", false
	}
	return slug, true
}

type productDetailResponse struct {
	ID          string                 `json:"id"`
	Slug        string                 `json:"slug"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	SaleStatus  string                 `json:"sale_status"`
	Categories  []categoryRefResponse  `json:"categories"`
	Variants    []variantOfferResponse `json:"variants"`
	Images      []catalog.ProductImage `json:"images"`
}

type categoryRefResponse struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type variantOfferResponse struct {
	ID          string `json:"id"`
	SKU         string `json:"sku"`
	Label       string `json:"label"`
	PriceCents  int64  `json:"price_cents"`
	Currency    string `json:"currency"`
	StockStatus string `json:"stock_status"`
}

func newProductDetailResponse(detail catalog.ProductDetail) productDetailResponse {
	response := productDetailResponse{
		ID: detail.ID, Slug: detail.Slug, Name: detail.Name, Description: detail.Description,
		SaleStatus: string(detail.SaleStatus),
		Categories: make([]categoryRefResponse, 0, len(detail.Categories)),
		Variants:   make([]variantOfferResponse, 0, len(detail.Variants)),
		Images:     detail.Images,
	}
	for _, category := range detail.Categories {
		response.Categories = append(response.Categories, categoryRefResponse{
			ID: category.ID, Slug: category.Slug, Name: category.Name,
		})
	}
	for _, variant := range detail.Variants {
		response.Variants = append(response.Variants, variantOfferResponse{
			ID: variant.ID, SKU: variant.SKU, Label: variant.Label, PriceCents: variant.PriceCents,
			Currency: variant.Currency, StockStatus: string(variant.StockStatus),
		})
	}
	return response
}
