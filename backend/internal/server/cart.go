package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/cart"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
)

type cartHandler struct{ repository *cart.Repository }

func RegisterCartRoutes(router *gin.Engine, auth *AuthHandler, repository *cart.Repository) {
	handler := &cartHandler{repository: repository}
	group := router.Group("/api/v1/cart", auth.RequireSession(), auth.RequireRole(identity.CustomerRole))
	group.GET("", handler.get)
	group.POST("/items", auth.RequireCSRF(), handler.add)
	group.PUT("/items/:variantId", auth.RequireCSRF(), handler.set)
	group.DELETE("/items/:variantId", auth.RequireCSRF(), handler.remove)
	group.PUT("/discount", auth.RequireCSRF(), handler.applyDiscount)
	group.DELETE("/discount", auth.RequireCSRF(), handler.removeDiscount)
}

type cartDiscountInput struct {
	Code string `json:"code"`
}

type cartQuantityInput struct {
	Quantity int32 `json:"quantity"`
}

type cartAddInput struct {
	VariantID string `json:"variant_id"`
	Quantity  int32  `json:"quantity"`
}

func validCartQuantity(value int32) bool { return value >= 1 && value <= cart.MaxQuantity }

func cartError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, cart.ErrNotFound), errors.Is(err, cart.ErrVariantNotFound), errors.Is(err, cart.ErrDiscountInvalid):
		writeProblem(c, NotFound)
	case errors.Is(err, cart.ErrRateLimited):
		writeProblem(c, RateLimited)
	case errors.Is(err, cart.ErrUnavailable), errors.Is(err, cart.ErrCartLimit):
		writeProblem(c, Conflict)
	default:
		HandleError(c, err)
	}
}

func (h *cartHandler) snapshot(c *gin.Context, status int) {
	result, err := h.repository.Get(c.Request.Context(), CurrentSession(c).Customer.ID)
	if err != nil {
		cartError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(status, result)
}

func (h *cartHandler) get(c *gin.Context) { h.snapshot(c, http.StatusOK) }

func (h *cartHandler) add(c *gin.Context) {
	var input cartAddInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	if !uuidPattern.MatchString(input.VariantID) || !validCartQuantity(input.Quantity) {
		writeProblem(c, InvalidRequest)
		return
	}
	if err := h.repository.Add(c.Request.Context(), CurrentSession(c).Customer.ID, input.VariantID, input.Quantity); err != nil {
		cartError(c, err)
		return
	}
	h.snapshot(c, http.StatusOK)
}

func (h *cartHandler) set(c *gin.Context) {
	variantID, ok := adminID(c, "variantId")
	if !ok {
		return
	}
	var input cartQuantityInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	if !validCartQuantity(input.Quantity) {
		writeProblem(c, InvalidRequest)
		return
	}
	if err := h.repository.Set(c.Request.Context(), CurrentSession(c).Customer.ID, variantID, input.Quantity); err != nil {
		cartError(c, err)
		return
	}
	h.snapshot(c, http.StatusOK)
}

func (h *cartHandler) remove(c *gin.Context) {
	variantID, ok := adminID(c, "variantId")
	if !ok {
		return
	}
	if err := h.repository.Remove(c.Request.Context(), CurrentSession(c).Customer.ID, variantID); err != nil {
		cartError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusNoContent)
}

// applyDiscount hinterlegt einen Rabattcode. Unbekannte, deaktivierte und abgelaufene Codes
// ergeben einheitlich 404, damit gültige Codes nicht erraten werden können.
func (h *cartHandler) applyDiscount(c *gin.Context) {
	var input cartDiscountInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	if len(input.Code) > 64 {
		writeProblem(c, InvalidRequest)
		return
	}
	if err := h.repository.ApplyDiscount(c.Request.Context(), CurrentSession(c).Customer.ID, input.Code); err != nil {
		cartError(c, err)
		return
	}
	h.snapshot(c, http.StatusOK)
}

func (h *cartHandler) removeDiscount(c *gin.Context) {
	if err := h.repository.RemoveDiscount(c.Request.Context(), CurrentSession(c).Customer.ID); err != nil {
		cartError(c, err)
		return
	}
	h.snapshot(c, http.StatusOK)
}
