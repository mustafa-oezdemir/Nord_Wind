package server

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type adminHandler struct{ repository *catalog.AdminRepository }

func RegisterAdminRoutes(router *gin.Engine, auth *AuthHandler, repository *catalog.AdminRepository) {
	h := &adminHandler{repository: repository}
	group := router.Group("/api/v1/admin", auth.RequireSession(), auth.RequireRole(identity.AdminRole))
	group.GET("/categories", h.categories)
	group.POST("/categories", auth.RequireCSRF(), h.createCategory)
	group.GET("/categories/:id", h.category)
	group.PUT("/categories/:id", auth.RequireCSRF(), h.updateCategory)
	group.DELETE("/categories/:id", auth.RequireCSRF(), h.deleteCategory)
	group.GET("/products", h.products)
	group.POST("/products", auth.RequireCSRF(), h.createProduct)
	group.GET("/products/:id", h.product)
	group.PUT("/products/:id", auth.RequireCSRF(), h.updateProduct)
	group.DELETE("/products/:id", auth.RequireCSRF(), h.deleteProduct)
	group.GET("/products/:id/categories", h.productCategories)
	group.POST("/products/:id/categories", auth.RequireCSRF(), h.attachCategory)
	group.DELETE("/products/:id/categories/:categoryId", auth.RequireCSRF(), h.detachCategory)
	group.GET("/products/:id/variants", h.variants)
	group.POST("/products/:id/variants", auth.RequireCSRF(), h.createVariant)
	group.GET("/variants/:id", h.variant)
	group.PUT("/variants/:id", auth.RequireCSRF(), h.updateVariant)
	group.DELETE("/variants/:id", auth.RequireCSRF(), h.deleteVariant)
	group.GET("/audit", h.audit)
	group.GET("/products/:id/images", h.images)
	group.POST("/products/:id/images", auth.RequireCSRF(), h.uploadImage)
	group.DELETE("/images/:id", auth.RequireCSRF(), h.deleteImage)
}

type categoryInput struct {
	ParentID    *string                `json:"parent_id"`
	Name        string                 `json:"name"`
	Slug        string                 `json:"slug"`
	Description string                 `json:"description"`
	Status      catalog.CategoryStatus `json:"status"`
	SortOrder   int32                  `json:"sort_order"`
}

func (v categoryInput) model(id string) (catalog.Category, bool) {
	item := catalog.Category{ID: id, ParentID: v.ParentID, Name: strings.TrimSpace(v.Name),
		Slug: strings.TrimSpace(v.Slug), Description: v.Description, Status: v.Status, SortOrder: v.SortOrder}
	if item.Status == "" {
		item.Status = catalog.CategoryActive
	}
	return item, validText(item.Name, 200) && validSlug(item.Slug) && len(item.Description) <= 5000 &&
		(item.Status == catalog.CategoryActive || item.Status == catalog.CategoryInactive) &&
		item.SortOrder >= 0 && (item.ParentID == nil || uuidPattern.MatchString(*item.ParentID)) &&
		(item.ParentID == nil || *item.ParentID != id)
}

type productInput struct {
	Name        string                `json:"name"`
	Slug        string                `json:"slug"`
	Description string                `json:"description"`
	Status      catalog.ProductStatus `json:"status"`
}

func (v productInput) model(id string) (catalog.Product, bool) {
	item := catalog.Product{ID: id, Name: strings.TrimSpace(v.Name), Slug: strings.TrimSpace(v.Slug),
		Description: v.Description, Status: v.Status}
	if item.Status == "" {
		item.Status = catalog.ProductDraft
	}
	return item, validText(item.Name, 200) && validSlug(item.Slug) && len(item.Description) <= 10000 &&
		(item.Status == catalog.ProductDraft || item.Status == catalog.ProductPublished || item.Status == catalog.ProductArchived)
}

type variantInput struct {
	SKU           string `json:"sku"`
	Label         string `json:"label"`
	PriceCents    int64  `json:"price_cents"`
	StockQuantity int32  `json:"stock_quantity"`
	IsActive      bool   `json:"is_active"`
}

func (v variantInput) model(id, productID string) (catalog.ProductVariant, bool) {
	item := catalog.ProductVariant{ID: id, ProductID: productID, SKU: strings.TrimSpace(v.SKU),
		Label: strings.TrimSpace(v.Label), PriceCents: v.PriceCents, Currency: "EUR",
		StockQuantity: v.StockQuantity, IsActive: v.IsActive}
	return item, validText(item.SKU, 120) && !strings.ContainsAny(item.SKU, " \t\r\n") &&
		validText(item.Label, 200) && item.PriceCents >= 0 && item.StockQuantity >= 0
}

func validText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func validSlug(value string) bool { return len(value) <= 120 && slugPattern.MatchString(value) }

func adminID(c *gin.Context, name string) (string, bool) {
	id := c.Param(name)
	if !uuidPattern.MatchString(id) {
		writeProblem(c, InvalidRequest)
		return "", false
	}
	return id, true
}

func adminError(c *gin.Context, err error) {
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		writeProblem(c, NotFound)
	case errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503" || pgErr.Code == "23001"):
		writeProblem(c, Conflict)
	case errors.As(err, &pgErr) && (pgErr.Code == "23514" || pgErr.Code == "22P02"):
		writeProblem(c, InvalidRequest)
	default:
		HandleError(c, err)
	}
}

func adminActor(c *gin.Context) string { return CurrentSession(c).Customer.ID }

func adminJSON(c *gin.Context, status int, value any) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, value)
}

func (h *adminHandler) categories(c *gin.Context) {
	items, err := h.repository.ListCategories(c.Request.Context())
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, gin.H{"data": items})
}

func (h *adminHandler) category(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	item, err := h.repository.GetCategory(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, item)
}

func (h *adminHandler) createCategory(c *gin.Context) {
	var input categoryInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	item, valid := input.model("")
	if !valid {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.CreateCategory(c.Request.Context(), adminActor(c), item)
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusCreated, result)
}

func (h *adminHandler) updateCategory(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	var input categoryInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	item, valid := input.model(id)
	if !valid {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.UpdateCategory(c.Request.Context(), adminActor(c), item)
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, result)
}

func (h *adminHandler) deleteCategory(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	if err := h.repository.DeleteCategory(c.Request.Context(), adminActor(c), id); err != nil {
		adminError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *adminHandler) products(c *gin.Context) {
	items, err := h.repository.ListProducts(c.Request.Context())
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, gin.H{"data": items})
}

func (h *adminHandler) product(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	item, err := h.repository.GetProduct(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, item)
}

func (h *adminHandler) createProduct(c *gin.Context) {
	var input productInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	item, valid := input.model("")
	if !valid {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.CreateProduct(c.Request.Context(), adminActor(c), item)
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusCreated, result)
}

func (h *adminHandler) updateProduct(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	var input productInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	item, valid := input.model(id)
	if !valid {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.UpdateProduct(c.Request.Context(), adminActor(c), item)
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, result)
}

func (h *adminHandler) deleteProduct(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	if err := h.repository.DeleteProduct(c.Request.Context(), adminActor(c), id); err != nil {
		adminError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *adminHandler) productCategories(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	if _, err := h.repository.GetProduct(c.Request.Context(), id); err != nil {
		adminError(c, err)
		return
	}
	items, err := h.repository.ListProductCategories(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	if items == nil {
		items = []catalog.Category{}
	}
	adminJSON(c, http.StatusOK, gin.H{"data": items})
}

func (h *adminHandler) attachCategory(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	var input struct {
		CategoryID string `json:"category_id"`
	}
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	if !uuidPattern.MatchString(input.CategoryID) {
		writeProblem(c, InvalidRequest)
		return
	}
	if err := h.repository.AttachCategory(c.Request.Context(), adminActor(c), id, input.CategoryID); err != nil {
		adminError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *adminHandler) detachCategory(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	categoryID, ok := adminID(c, "categoryId")
	if !ok {
		return
	}
	if err := h.repository.DetachCategory(c.Request.Context(), adminActor(c), id, categoryID); err != nil {
		adminError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *adminHandler) variants(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	if _, err := h.repository.GetProduct(c.Request.Context(), id); err != nil {
		adminError(c, err)
		return
	}
	items, err := h.repository.ListVariants(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	if items == nil {
		items = []catalog.ProductVariant{}
	}
	adminJSON(c, http.StatusOK, gin.H{"data": items})
}

func (h *adminHandler) variant(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	item, err := h.repository.GetVariant(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, item)
}

func (h *adminHandler) createVariant(c *gin.Context) {
	productID, ok := adminID(c, "id")
	if !ok {
		return
	}
	var input variantInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	item, valid := input.model("", productID)
	if !valid {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.CreateVariant(c.Request.Context(), adminActor(c), item)
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusCreated, result)
}

func (h *adminHandler) updateVariant(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	existing, err := h.repository.GetVariant(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	var input variantInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	item, valid := input.model(id, existing.ProductID)
	if !valid {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.UpdateVariant(c.Request.Context(), adminActor(c), item)
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, result)
}

func (h *adminHandler) deleteVariant(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	if err := h.repository.DeleteVariant(c.Request.Context(), adminActor(c), id); err != nil {
		adminError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *adminHandler) audit(c *gin.Context) {
	items, err := h.repository.ListAudit(c.Request.Context())
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, gin.H{"data": items})
}
