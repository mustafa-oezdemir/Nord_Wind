package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/inventory"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/payment"
)

type mockPaymentHandler struct {
	repository *payment.Repository
	secret     []byte
}

// RegisterMockPaymentRoutes wird ausschließlich im Entwicklungsbetrieb aufgerufen.
func RegisterMockPaymentRoutes(router *gin.Engine, auth *AuthHandler, repository *payment.Repository, secret []byte) {
	handler := &mockPaymentHandler{repository: repository, secret: secret}
	group := router.Group("/api/v1/mock-payments")
	group.GET("/methods", handler.methods)
	group.POST("/webhook", handler.webhook)
	protected := group.Group("", auth.RequireSession(), auth.RequireRole(identity.CustomerRole))
	protected.POST("", auth.RequireCSRF(), handler.initiate)
	protected.GET("/by-order/:orderId", handler.getByOrder)
	protected.GET("/:paymentId", handler.get)
	protected.POST("/:paymentId/simulate", auth.RequireCSRF(), handler.simulate)
}

type mockInitiateInput struct {
	OrderID string         `json:"order_id"`
	Method  payment.Method `json:"method"`
}

type mockSimulateInput struct {
	Outcome payment.Outcome `json:"outcome"`
}

func (h *mockPaymentHandler) methods(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"mode": "mock", "methods": []payment.Method{
		payment.DebitCard, payment.Klarna, payment.PayPal, payment.BankTransfer,
	}})
}

func (h *mockPaymentHandler) initiate(c *gin.Context) {
	var input mockInitiateInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	if !uuidPattern.MatchString(input.OrderID) || !input.Method.Valid() {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.Initiate(c.Request.Context(), CurrentSession(c).Customer.ID, input.OrderID, input.Method)
	if err != nil {
		observePayment(c, "", err)
		h.writeError(c, err)
		return
	}
	observePayment(c, "pending", nil)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *mockPaymentHandler) get(c *gin.Context) {
	paymentID := c.Param("paymentId")
	if len(paymentID) != 36 {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.Get(c.Request.Context(), CurrentSession(c).Customer.ID, paymentID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *mockPaymentHandler) getByOrder(c *gin.Context) {
	orderID := c.Param("orderId")
	if !uuidPattern.MatchString(orderID) {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.GetByOrder(c.Request.Context(), CurrentSession(c).Customer.ID, orderID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *mockPaymentHandler) simulate(c *gin.Context) {
	var input mockSimulateInput
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	if !input.Outcome.Valid() {
		writeProblem(c, InvalidRequest)
		return
	}
	current, err := h.repository.Get(c.Request.Context(), CurrentSession(c).Customer.ID, c.Param("paymentId"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if (input.Outcome == payment.Refunded && current.State != "succeeded") ||
		(input.Outcome != payment.Refunded && current.State != "pending") {
		writeProblem(c, Conflict)
		return
	}
	eventID, err := payment.NewID("evt_")
	if err != nil {
		HandleError(c, err)
		return
	}
	event := payment.Event{ID: eventID, PaymentID: current.ID, OrderID: current.OrderID,
		Method: current.Method, Outcome: input.Outcome, AmountCents: current.AmountCents, Currency: current.Currency}
	raw, err := payment.MarshalEvent(event)
	if err != nil {
		HandleError(c, err)
		return
	}
	now := time.Now()
	if err := h.verifyAndProcess(c, raw, payment.Sign(h.secret, raw, now), now); err != nil {
		return
	}
	updated, err := h.repository.Get(c.Request.Context(), CurrentSession(c).Customer.ID, current.ID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"event_id": eventID, "payment": updated})
}

func (h *mockPaymentHandler) webhook(c *gin.Context) {
	if c.ContentType() != "application/json" {
		observePayment(c, "", payment.ErrInvalidEvent)
		writeProblem(c, InvalidRequest)
		return
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		observePayment(c, "", err)
		HandleError(c, err)
		return
	}
	if err := h.verifyAndProcess(c, raw, c.GetHeader("X-Nordwinds-Mock-Signature"), time.Now()); err != nil {
		return
	}
	c.JSON(http.StatusOK, gin.H{"received": true})
}

func (h *mockPaymentHandler) verifyAndProcess(c *gin.Context, raw []byte, signature string, now time.Time) error {
	if err := payment.Verify(h.secret, signature, raw, now); err != nil {
		observePayment(c, "", err)
		writeProblem(c, Unauthenticated)
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var event payment.Event
	if err := decoder.Decode(&event); err != nil || !event.Valid() {
		observePayment(c, "", payment.ErrInvalidEvent)
		writeProblem(c, InvalidRequest)
		return payment.ErrInvalidEvent
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		observePayment(c, "", payment.ErrInvalidEvent)
		writeProblem(c, InvalidRequest)
		return payment.ErrInvalidEvent
	}
	if err := h.repository.Process(c.Request.Context(), event, raw); err != nil {
		observePayment(c, "", err)
		h.writeError(c, err)
		return err
	}
	observePayment(c, string(event.Outcome), nil)
	return nil
}

func (h *mockPaymentHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, payment.ErrNotFound):
		writeProblem(c, NotFound)
	case errors.Is(err, payment.ErrConflict), errors.Is(err, inventory.ErrExpired), errors.Is(err, inventory.ErrState):
		writeProblem(c, Conflict)
	case errors.Is(err, payment.ErrInvalidEvent):
		writeProblem(c, InvalidRequest)
	default:
		HandleError(c, err)
	}
}
