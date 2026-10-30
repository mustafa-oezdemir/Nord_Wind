package server

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
)

// RegisterCatalogRoutes registriert die öffentlichen, lesenden Katalog-Endpunkte.
func RegisterCatalogRoutes(router *gin.Engine, repository *catalog.Repository) {
	router.GET("/api/v1/products", func(c *gin.Context) {
		options, page, err := catalogOptions(c)
		if err != nil {
			writeCatalogProblem(c, http.StatusBadRequest)
			return
		}
		result, err := repository.ListProducts(c.Request.Context(), options)
		if err != nil {
			writeCatalogError(c, err)
			return
		}
		products := make([]productResponse, 0, len(result.Items))
		for _, product := range result.Items {
			products = append(products, productResponse{
				ID: product.ID, Slug: product.Slug, Name: product.Name,
				Description: product.Description, PriceCents: product.PriceCents,
				Currency: product.Currency, Available: product.Available,
				VariantCount: product.VariantCount,
			})
		}
		c.JSON(http.StatusOK, listResponse[productResponse]{
			Data: products, Pagination: pagination(page, options.Limit, result.Total),
		})
	})
	router.GET("/api/v1/categories", func(c *gin.Context) {
		options, page, err := catalogOptions(c)
		if err != nil {
			writeCatalogProblem(c, http.StatusBadRequest)
			return
		}
		result, err := repository.ListCategories(c.Request.Context(), options)
		if err != nil {
			writeCatalogError(c, err)
			return
		}
		categories := make([]categoryResponse, 0, len(result.Items))
		for _, category := range result.Items {
			categories = append(categories, categoryResponse{
				ID: category.ID, ParentID: category.ParentID, Slug: category.Slug,
				Name: category.Name, Description: category.Description, SortOrder: category.SortOrder,
			})
		}
		c.JSON(http.StatusOK, listResponse[categoryResponse]{
			Data: categories, Pagination: pagination(page, options.Limit, result.Total),
		})
	})
	registerProductDetailRoute(router, repository)
}

func catalogOptions(c *gin.Context) (catalog.ListOptions, int, error) {
	options := catalog.ListOptions{Limit: 20}
	raw := c.Request.URL.RawQuery
	if len(raw) > 512 {
		return options, 0, catalog.ErrInvalidListOptions
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return options, 0, err
	}
	for key, value := range values {
		if len(value) != 1 {
			return options, 0, catalog.ErrInvalidListOptions
		}
		switch key {
		case "category", "q", "sort", "page", "limit":
		default:
			return options, 0, catalog.ErrInvalidListOptions
		}
	}
	category := strings.TrimSpace(values.Get("category"))
	if _, ok := values["category"]; ok && category == "" {
		return options, 0, catalog.ErrInvalidListOptions
	}
	query := strings.TrimSpace(values.Get("q"))
	if !utf8.ValidString(category) || !utf8.ValidString(query) ||
		strings.ContainsRune(category, 0) || strings.ContainsRune(query, 0) ||
		utf8.RuneCountInString(category) > 120 || utf8.RuneCountInString(query) > 100 {
		return options, 0, catalog.ErrInvalidListOptions
	}
	options.Category = category
	options.Query = query
	options.Sort = values.Get("sort")
	if len(options.Sort) > 32 {
		return options, 0, catalog.ErrInvalidListOptions
	}
	page := 1
	if _, ok := values["page"]; ok {
		page, err = strconv.Atoi(values.Get("page"))
		if err != nil || page < 1 || page > 10000 {
			return options, 0, catalog.ErrInvalidListOptions
		}
	}
	if _, ok := values["limit"]; ok {
		options.Limit, err = strconv.Atoi(values.Get("limit"))
		if err != nil || options.Limit < 1 || options.Limit > 100 {
			return options, 0, catalog.ErrInvalidListOptions
		}
	}
	options.Offset = (page - 1) * options.Limit
	return options, page, nil
}

type listResponse[T any] struct {
	Data       []T            `json:"data"`
	Pagination paginationInfo `json:"pagination"`
}

type paginationInfo struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

func pagination(page, limit int, total int64) paginationInfo {
	totalPages := int64(0)
	if total > 0 {
		totalPages = (total-1)/int64(limit) + 1
	}
	return paginationInfo{Page: page, Limit: limit, Total: total, TotalPages: totalPages}
}

type productResponse struct {
	ID           string `json:"id"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	PriceCents   int64  `json:"price_cents"`
	Currency     string `json:"currency"`
	Available    bool   `json:"available"`
	VariantCount int64  `json:"variant_count"`
}

type categoryResponse struct {
	ID          string  `json:"id"`
	ParentID    *string `json:"parent_id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	SortOrder   int32   `json:"sort_order"`
}

func writeCatalogError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		writeCatalogProblem(c, http.StatusNotFound)
	case errors.Is(err, catalog.ErrInvalidListOptions):
		writeCatalogProblem(c, http.StatusBadRequest)
	default:
		writeCatalogProblem(c, http.StatusInternalServerError)
	}
}

func writeCatalogProblem(c *gin.Context, status int) {
	switch status {
	case http.StatusBadRequest:
		HandleError(c, &APIError{Code: InvalidRequest})
	case http.StatusNotFound:
		HandleError(c, &APIError{Code: NotFound})
	default:
		HandleError(c, &APIError{Code: InternalError})
	}
}
