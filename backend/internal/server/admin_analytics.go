package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/analytics"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
)

// RegisterAdminAnalyticsRoutes liefert nur aggregierte Shopdaten an Admins.
func RegisterAdminAnalyticsRoutes(router *gin.Engine, auth *AuthHandler, repository *analytics.Repository) {
	group := router.Group("/api/v1/admin/analytics", auth.RequireSession(), auth.RequireRole(identity.AdminRole))
	group.GET("/sales", func(c *gin.Context) {
		groupBy := analytics.GroupBy(c.DefaultQuery("group_by", "total"))
		period := analytics.Period(c.DefaultQuery("period", "week"))
		if !analytics.ValidGroupBy(groupBy) || !analytics.ValidPeriod(period) {
			writeProblem(c, InvalidRequest)
			return
		}
		report, err := repository.Sales(c.Request.Context(), groupBy, period)
		if err != nil {
			HandleError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, report)
	})
}
