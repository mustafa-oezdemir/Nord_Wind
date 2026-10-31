package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

// Ein noch referenzierter Datensatz meldet je nach PostgreSQL-Version 23001 (ab 18,
// ON DELETE RESTRICT) oder 23503 (bis 17); beide müssen als 409 ankommen.
func TestAdminErrorTreatsReferencedRowsAsConflictOnEveryPostgresVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for code, want := range map[string]int{
		"23001": http.StatusConflict,
		"23503": http.StatusConflict,
		"23505": http.StatusConflict,
		"23514": http.StatusBadRequest,
	} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/x", nil)
		adminError(c, &pgconn.PgError{Code: code, Message: "intern"})
		if response.Code != want {
			t.Errorf("SQLSTATE %s: %d, erwartet %d", code, response.Code, want)
		}
	}
}
