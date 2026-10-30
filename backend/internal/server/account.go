package server

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
)

var addressIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type accountHandler struct {
	repository *orders.Repository
	auth       *AuthHandler
}

func RegisterAccountRoutes(router *gin.Engine, auth *AuthHandler, repository *orders.Repository) {
	h := &accountHandler{repository: repository, auth: auth}
	group := router.Group("/api/v1/customer", auth.RequireSession(), auth.RequireRole(identity.CustomerRole))
	group.GET("/addresses", h.addresses)
	group.POST("/addresses", auth.RequireCSRF(), h.createAddress)
	group.PUT("/addresses/:id", auth.RequireCSRF(), h.updateAddress)
	group.DELETE("/addresses/:id", auth.RequireCSRF(), h.deleteAddress)
	group.GET("/export", h.export)
	group.DELETE("/me", auth.RequireCSRF(), h.close)
}

func (h *accountHandler) addresses(c *gin.Context) {
	data, err := h.repository.Addresses(c.Request.Context(), CurrentSession(c).Customer.ID)
	if err != nil {
		HandleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": data})
}

type addressRequest struct {
	Label   string         `json:"label"`
	Address orders.Address `json:"address"`
}

func (h *accountHandler) writeAddress(c *gin.Context, id string) {
	var input addressRequest
	if err := readJSON(c, &input); err != nil {
		HandleError(c, err)
		return
	}
	address := input.Address.Normalize()
	label := strings.TrimSpace(input.Label)
	if fields := address.Validate(); len(fields) > 0 {
		writeProblemWith(c, ValidationFailed, problemExtension{Errors: fields})
		return
	}
	if len([]rune(label)) < 1 || len([]rune(label)) > 60 || strings.IndexFunc(label, unicode.IsControl) >= 0 {
		writeProblem(c, ValidationFailed)
		return
	}
	result, err := h.repository.SaveAddress(c.Request.Context(), CurrentSession(c).Customer.ID, id, label, address)
	if errors.Is(err, orders.ErrNotFound) {
		writeProblem(c, NotFound)
		return
	}
	if err != nil {
		HandleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	if id == "" {
		c.JSON(http.StatusCreated, result)
	} else {
		c.JSON(http.StatusOK, result)
	}
}

func (h *accountHandler) createAddress(c *gin.Context) { h.writeAddress(c, "") }
func (h *accountHandler) updateAddress(c *gin.Context) {
	if !addressIDPattern.MatchString(c.Param("id")) {
		writeProblem(c, NotFound)
		return
	}
	h.writeAddress(c, c.Param("id"))
}
func (h *accountHandler) deleteAddress(c *gin.Context) {
	if !addressIDPattern.MatchString(c.Param("id")) {
		writeProblem(c, NotFound)
		return
	}
	err := h.repository.DeleteAddress(c.Request.Context(), CurrentSession(c).Customer.ID, c.Param("id"))
	if errors.Is(err, orders.ErrNotFound) {
		writeProblem(c, NotFound)
		return
	}
	if err != nil {
		HandleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *accountHandler) export(c *gin.Context) {
	data, err := h.repository.Export(c.Request.Context(), CurrentSession(c).Customer.ID)
	if err != nil {
		HandleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", `attachment; filename="nordwinds-konto.json"`)
	c.JSON(http.StatusOK, data)
}

func (h *accountHandler) close(c *gin.Context) {
	err := h.repository.CloseAccount(c.Request.Context(), CurrentSession(c).Customer.ID)
	if errors.Is(err, orders.ErrOpenOrders) {
		writeProblem(c, Conflict)
		return
	}
	if errors.Is(err, orders.ErrNotFound) {
		writeProblem(c, NotFound)
		return
	}
	if err != nil {
		HandleError(c, err)
		return
	}
	h.auth.setCookie(c, "", true)
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusNoContent)
}
