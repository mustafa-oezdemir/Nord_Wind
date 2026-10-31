package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/analytics"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
)

// RegisterAdminAnalyticsRoutes liefert nur aggregierte Shopdaten an Admins.
func RegisterAdminAnalyticsRoutes(router *gin.Engine, auth *AuthHandler, repository *analytics.Repository) {
	group := router.Group("/api/v1/admin/analytics", auth.RequireSession(), auth.RequireRole(identity.AdminRole))
	group.GET("/sales", func(c *gin.Context) {
		query := analytics.Query{
			GroupBy:    analytics.GroupBy(c.DefaultQuery("group_by", "total")),
			Period:     analytics.Period(c.DefaultQuery("period", "week")),
			From:       c.Query("from"),
			To:         c.Query("to"),
			CategoryID: c.Query("category_id"),
			ProductID:  c.Query("product_id"),
		}
		if !analytics.ValidGroupBy(query.GroupBy) || !analytics.ValidPeriod(query.Period) ||
			!optionalUUID(query.CategoryID) || !optionalUUID(query.ProductID) {
			writeProblem(c, InvalidRequest)
			return
		}
		report, err := repository.Sales(c.Request.Context(), query)
		switch {
		case errors.Is(err, analytics.ErrInvalidQuery):
			writeProblem(c, InvalidRequest)
			return
		case errors.Is(err, analytics.ErrNotFound):
			writeProblem(c, NotFound)
			return
		case err != nil:
			HandleError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, report)
	})
	// Kategorien und Produkte für die Auswahllisten der Analyse.
	group.GET("/catalog", func(c *gin.Context) {
		catalog, err := repository.Catalog(c.Request.Context())
		if err != nil {
			HandleError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, catalog)
	})
}

func optionalUUID(value string) bool { return value == "" || uuidPattern.MatchString(value) }
