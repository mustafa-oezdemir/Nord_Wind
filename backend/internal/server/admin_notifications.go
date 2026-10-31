package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/notification"
)

type adminNotificationHandler struct {
	store  *notification.Store
	logger *slog.Logger
}

// RegisterAdminNotificationRoutes zeigt Admins den Zustand der E-Mail-Outbox ohne
// Empfängeradressen und Inhalte und erlaubt, fehlgeschlagene Einträge erneut anzustoßen.
func RegisterAdminNotificationRoutes(router *gin.Engine, auth *AuthHandler, store *notification.Store, logger *slog.Logger) {
	h := &adminNotificationHandler{store: store, logger: logger}
	group := router.Group("/api/v1/admin/notifications", auth.RequireSession(), auth.RequireRole(identity.AdminRole))
	group.GET("", h.list)
	group.POST("/:id/retry", auth.RequireCSRF(), h.retry)
}

func (h *adminNotificationHandler) list(c *gin.Context) {
	state := c.Query("state")
	if state != "" && !notification.ValidState(state) {
		writeProblem(c, InvalidRequest)
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeProblem(c, InvalidRequest)
			return
		}
		limit = parsed
	}
	entries, err := h.store.List(c.Request.Context(), state, limit)
	if err != nil {
		HandleError(c, err)
		return
	}
	counts, err := h.store.Counts(c.Request.Context())
	if err != nil {
		HandleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": entries, "counts": counts})
}

func (h *adminNotificationHandler) retry(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		writeProblem(c, InvalidRequest)
		return
	}
	switch err := h.store.Retry(c.Request.Context(), id); {
	case errors.Is(err, notification.ErrNotFound):
		writeProblem(c, NotFound)
	case errors.Is(err, notification.ErrNotRetryable):
		writeProblem(c, Conflict)
	case err != nil:
		HandleError(c, err)
	default:
		if h.logger != nil {
			h.logger.Info("Benachrichtigung erneut eingeplant", "event", "notification.retry",
				"notification_id", id, "actor_customer_id", CurrentSession(c).Customer.ID)
		}
		c.Status(http.StatusNoContent)
	}
}
