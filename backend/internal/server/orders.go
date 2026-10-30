package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/inventory"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
)

type ordersHandler struct{ repository *orders.Repository }

// RegisterOrderRoutes registriert die Kasse (ADR-008). Nur Kundensessions dürfen bestellen;
// das Anlegen verlangt CSRF und einen Idempotency-Key.
func RegisterOrderRoutes(router *gin.Engine, auth *AuthHandler, repository *orders.Repository) {
	handler := &ordersHandler{repository: repository}
	group := router.Group("/api/v1/orders", auth.RequireSession(), auth.RequireRole(identity.CustomerRole))
	group.POST("", auth.RequireCSRF(), handler.create)
	group.GET("/:orderNumber", handler.get)
	group.POST("/:orderNumber/cancel", auth.RequireCSRF(), handler.cancel)
}

type orderCreateInput struct {
	Address            *orders.Address `json:"address"`
	ExpectedTotalCents *int64          `json:"expected_total_cents"`
}

func (h *ordersHandler) create(c *gin.Context) {
	key := c.GetHeader("Idempotency-Key")
	if !orders.ValidIdempotencyKey(key) {
		writeProblem(c, InvalidRequest)
		return
	}
	var input orderCreateInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	if input.ExpectedTotalCents == nil || *input.ExpectedTotalCents < 0 {
		writeProblem(c, InvalidRequest)
		return
	}
	address := orders.Address{}
	if input.Address != nil {
		address = input.Address.Normalize()
	}
	if fieldErrors := address.Validate(); len(fieldErrors) > 0 {
		writeProblemWith(c, ValidationFailed, problemExtension{Errors: fieldErrors})
		return
	}
	order, replayed, err := h.repository.Create(c.Request.Context(), orders.CreateInput{
		CustomerID: CurrentSession(c).Customer.ID, IdempotencyKey: key,
		Address: address, ExpectedTotalCents: *input.ExpectedTotalCents,
	})
	var changed *orders.CartChangedError
	switch {
	case errors.As(err, &changed):
		writeProblemWith(c, CartChanged, problemExtension{Reason: changed.Reason, Cart: changed.Cart})
		return
	case errors.Is(err, orders.ErrCartEmpty):
		writeProblem(c, CartEmpty)
		return
	case errors.Is(err, orders.ErrIdempotencyMismatch):
		writeProblem(c, IdempotencyKeyReused)
		return
	case err != nil:
		HandleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Location", "/api/v1/orders/"+order.OrderNumber)
	if replayed {
		c.Header("Idempotent-Replayed", "true")
		c.JSON(http.StatusOK, order)
		return
	}
	c.JSON(http.StatusCreated, order)
}

func (h *ordersHandler) get(c *gin.Context) {
	number := c.Param("orderNumber")
	if !orders.ValidOrderNumber(number) {
		writeProblem(c, NotFound)
		return
	}
	order, err := h.repository.Get(c.Request.Context(), CurrentSession(c).Customer.ID, number)
	if errors.Is(err, orders.ErrNotFound) {
		writeProblem(c, NotFound)
		return
	}
	if err != nil {
		HandleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, order)
}

func (h *ordersHandler) cancel(c *gin.Context) {
	number := c.Param("orderNumber")
	if !orders.ValidOrderNumber(number) {
		writeProblem(c, NotFound)
		return
	}
	order, err := h.repository.Cancel(c.Request.Context(), CurrentSession(c).Customer.ID, number)
	switch {
	case errors.Is(err, orders.ErrNotFound):
		writeProblem(c, NotFound)
	case errors.Is(err, inventory.ErrState):
		writeProblem(c, Conflict)
	case err != nil:
		HandleError(c, err)
	default:
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, order)
	}
}
