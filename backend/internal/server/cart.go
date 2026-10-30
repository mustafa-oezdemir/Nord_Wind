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
	case errors.Is(err, cart.ErrNotFound), errors.Is(err, cart.ErrVariantNotFound):
		writeProblem(c, NotFound)
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
