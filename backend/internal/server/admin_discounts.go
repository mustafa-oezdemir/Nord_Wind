package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/cart"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/pricing"
)

type discountAdminHandler struct{ repository *cart.DiscountAdmin }

// RegisterDiscountAdminRoutes registriert die Pflege prozentualer Rabattcodes (ADR-007).
func RegisterDiscountAdminRoutes(router *gin.Engine, auth *AuthHandler, repository *cart.DiscountAdmin) {
	h := &discountAdminHandler{repository: repository}
	group := router.Group("/api/v1/admin/discount-codes", auth.RequireSession(), auth.RequireRole(identity.AdminRole))
	group.GET("", h.list)
	group.POST("", auth.RequireCSRF(), h.create)
	group.PUT("/:id", auth.RequireCSRF(), h.update)
}

type discountCodeInput struct {
	Code       string     `json:"code"`
	PercentBP  int64      `json:"percent_bp"`
	IsActive   *bool      `json:"is_active"`
	ValidFrom  *time.Time `json:"valid_from"`
	ValidUntil *time.Time `json:"valid_until"`
}

func (v discountCodeInput) model(id string) (cart.DiscountCode, bool) {
	item := cart.DiscountCode{ID: id, Code: cart.NormalizeDiscountCode(v.Code), PercentBP: v.PercentBP,
		IsActive: v.IsActive == nil || *v.IsActive, ValidFrom: v.ValidFrom, ValidUntil: v.ValidUntil}
	validWindow := item.ValidFrom == nil || item.ValidUntil == nil || item.ValidUntil.After(*item.ValidFrom)
	return item, item.PercentBP >= 1 && item.PercentBP <= pricing.MaxDiscountBP && validWindow
}

func (h *discountAdminHandler) list(c *gin.Context) {
	items, err := h.repository.List(c.Request.Context())
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, gin.H{"data": items})
}

func (h *discountAdminHandler) create(c *gin.Context) {
	var input discountCodeInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	item, ok := input.model("")
	if !ok || !cart.ValidDiscountCode(item.Code) {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.Create(c.Request.Context(), adminActor(c), item)
	if err != nil {
		discountAdminError(c, err)
		return
	}
	adminJSON(c, http.StatusCreated, result)
}

func (h *discountAdminHandler) update(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	var input discountCodeInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	item, valid := input.model(id)
	if !valid || input.Code != "" {
		// Der Code ist nach dem Anlegen unveränderlich.
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.Update(c.Request.Context(), adminActor(c), item)
	if err != nil {
		discountAdminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, result)
}

func discountAdminError(c *gin.Context, err error) {
	if errors.Is(err, cart.ErrNotFound) {
		writeProblem(c, NotFound)
		return
	}
	adminError(c, err)
}
