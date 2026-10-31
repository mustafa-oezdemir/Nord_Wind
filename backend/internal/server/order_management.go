package server

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
)

type adminOrderHandler struct {
	repository *orders.Repository
	logger     *slog.Logger
}

func RegisterAdminOrderRoutes(router *gin.Engine, auth *AuthHandler, repository *orders.Repository, logger *slog.Logger) {
	h := &adminOrderHandler{repository: repository, logger: logger}
	group := router.Group("/api/v1/admin/orders", auth.RequireSession(), auth.RequireAnyRole(identity.AdminRole, identity.EmployeeRole))
	group.GET("", h.list)
	group.GET("/shipment-metrics", h.metrics)
	group.GET("/:orderNumber", h.get)
	group.PATCH("/:orderNumber/status", auth.RequireCSRF(), h.transition)
}

func (h *adminOrderHandler) list(c *gin.Context) {
	data, err := h.repository.List(c.Request.Context(), "")
	if err != nil {
		HandleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func (h *adminOrderHandler) get(c *gin.Context) {
	number := c.Param("orderNumber")
	if !orders.ValidOrderNumber(number) {
		writeProblem(c, NotFound)
		return
	}
	order, err := h.repository.AdminGet(c.Request.Context(), number)
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

type statusRequest struct {
	Status         orders.Status `json:"status"`
	TrackingNumber string        `json:"tracking_number"`
}

func (h *adminOrderHandler) transition(c *gin.Context) {
	number := c.Param("orderNumber")
	if !orders.ValidOrderNumber(number) {
		writeProblem(c, NotFound)
		return
	}
	var input statusRequest
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	session := CurrentSession(c)
	order, err := h.repository.StaffTransition(c.Request.Context(), number, input.Status, input.TrackingNumber,
		string(session.Customer.Role), session.Customer.ID)
	if err == nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, order)
		return
	}
	if errors.Is(err, orders.ErrNotFound) {
		writeProblem(c, NotFound)
		return
	}
	if errors.Is(err, orders.ErrInvalidTransition) {
		if input.Status == orders.Shipped {
			h.failure(c, number, "invalid_state")
		}
		writeProblem(c, Conflict)
		return
	}
	if errors.Is(err, orders.ErrInvalidTracking) {
		if input.Status == orders.Shipped {
			h.failure(c, number, "invalid_tracking")
		}
		writeProblem(c, ValidationFailed)
		return
	}
	if input.Status == orders.Shipped {
		h.failure(c, number, "shipment_failed")
	}
	HandleError(c, err)
}

func (h *adminOrderHandler) failure(c *gin.Context, number, reason string) {
	if h.logger != nil {
		h.logger.Error("Versandversuch fehlgeschlagen", "event", "shipment.failed", "reason", reason,
			"request_id", RequestID(c.Request.Context()))
	}
	if err := h.repository.RecordShipmentFailure(c.Request.Context(), number, reason); err != nil && h.logger != nil {
		h.logger.Error("Versandfehlerzähler konnte nicht geschrieben werden", "event", "shipment.metric.failed")
	}
}

func (h *adminOrderHandler) metrics(c *gin.Context) {
	data, err := h.repository.ShipmentMetrics(c.Request.Context())
	if err != nil {
		HandleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": data})
}
