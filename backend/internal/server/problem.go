package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorCode ist ein stabiler, maschinenlesbarer Fehlercode der API.
type ErrorCode string

const (
	InvalidRequest     ErrorCode = "INVALID_REQUEST"
	NotFound           ErrorCode = "NOT_FOUND"
	Conflict           ErrorCode = "CONFLICT"
	Unauthenticated    ErrorCode = "UNAUTHENTICATED"
	Forbidden          ErrorCode = "FORBIDDEN"
	RateLimited        ErrorCode = "RATE_LIMITED"
	BodyTooLarge       ErrorCode = "BODY_TOO_LARGE"
	URITooLong         ErrorCode = "URI_TOO_LONG"
	MethodNotAllowed   ErrorCode = "METHOD_NOT_ALLOWED"
	RequestTimeout     ErrorCode = "REQUEST_TIMEOUT"
	ServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	InternalError      ErrorCode = "INTERNAL_ERROR"
	// Kasse (NW-16)
	ValidationFailed     ErrorCode = "VALIDATION_FAILED"
	CartEmpty            ErrorCode = "CART_EMPTY"
	CartChanged          ErrorCode = "CART_CHANGED"
	IdempotencyKeyReused ErrorCode = "IDEMPOTENCY_KEY_REUSED"
)

// APIError transportiert nur einen bekannten Fehlercode, nie sensible Ursachen.
type APIError struct{ Code ErrorCode }

func (e *APIError) Error() string { return string(e.Code) }

type problemSpec struct {
	status  int
	typeURI string
	title   string
	detail  string
}

var problemSpecs = map[ErrorCode]problemSpec{
	InvalidRequest:     {http.StatusBadRequest, "/api/v1/problems/invalid-request", "Ungültige Anfrage", "Die Anfrage konnte nicht verarbeitet werden."},
	NotFound:           {http.StatusNotFound, "/api/v1/problems/not-found", "Nicht gefunden", "Die angeforderte Ressource wurde nicht gefunden."},
	Conflict:           {http.StatusConflict, "/api/v1/problems/conflict", "Konflikt", "Die Anfrage widerspricht dem aktuellen Zustand."},
	Unauthenticated:    {http.StatusUnauthorized, "/api/v1/problems/unauthenticated", "Nicht angemeldet", "Eine gültige Anmeldung ist erforderlich."},
	Forbidden:          {http.StatusForbidden, "/api/v1/problems/forbidden", "Zugriff verweigert", "Für diese Aktion fehlt die Berechtigung."},
	RateLimited:        {http.StatusTooManyRequests, "/api/v1/problems/rate-limited", "Zu viele Anfragen", "Bitte versuchen Sie es später erneut."},
	BodyTooLarge:       {http.StatusRequestEntityTooLarge, "/api/v1/problems/body-too-large", "Anfrage zu groß", "Der Anfrageinhalt überschreitet die zulässige Größe."},
	URITooLong:         {http.StatusRequestURITooLong, "/api/v1/problems/uri-too-long", "Adresse zu lang", "Die Anfrageadresse überschreitet die zulässige Länge."},
	MethodNotAllowed:   {http.StatusMethodNotAllowed, "/api/v1/problems/method-not-allowed", "Methode nicht erlaubt", "Diese HTTP-Methode ist hier nicht erlaubt."},
	RequestTimeout:     {http.StatusServiceUnavailable, "/api/v1/problems/request-timeout", "Zeitüberschreitung", "Die Anfrage konnte nicht rechtzeitig bearbeitet werden."},
	ServiceUnavailable: {http.StatusServiceUnavailable, "/api/v1/problems/service-unavailable", "Dienst nicht verfügbar", "Der Dienst ist vorübergehend nicht verfügbar."},
	InternalError:      {http.StatusInternalServerError, "/api/v1/problems/internal-error", "Interner Fehler", "Die Anfrage konnte nicht bearbeitet werden."},
	ValidationFailed:   {http.StatusUnprocessableEntity, "/api/v1/problems/validation-failed", "Eingaben unvollständig", "Bitte prüfen Sie die markierten Felder."},
	CartEmpty:          {http.StatusConflict, "/api/v1/problems/cart-empty", "Warenkorb leer", "Der Warenkorb enthält keine Artikel."},
	CartChanged:        {http.StatusConflict, "/api/v1/problems/cart-changed", "Warenkorb geändert", "Preis, Bestand oder Rabatt haben sich geändert. Bitte prüfen Sie die neue Summe."},
	IdempotencyKeyReused: {http.StatusUnprocessableEntity, "/api/v1/problems/idempotency-key-reused", "Schlüssel bereits verwendet",
		"Dieser Idempotency-Key gehört zu einer anderen Anfrage."},
}

type problem struct {
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Status    int       `json:"status"`
	Detail    string    `json:"detail"`
	Code      ErrorCode `json:"code"`
	RequestID string    `json:"request_id"`
	problemExtension
}

// problemExtension sind optionale Zusatzfelder nach RFC 9457 Abschnitt 3.2.
type problemExtension struct {
	// Errors nennt ungültige Felder mit stabilen Codes, nie die eingegebenen Werte.
	Errors any `json:"errors,omitempty"`
	// Reason präzisiert CART_CHANGED (total_changed, item_unavailable).
	Reason string `json:"reason,omitempty"`
	// Cart ist der aktuelle Warenkorb des angemeldeten Kunden bei CART_CHANGED.
	Cart any `json:"cart,omitempty"`
}

// HandleError ordnet bekannte Fehler einer sicheren Problemantwort zu.
func HandleError(c *gin.Context, err error) {
	var apiErr *APIError
	var maxBytesErr *http.MaxBytesError
	switch {
	case errors.As(err, &apiErr):
		writeProblem(c, apiErr.Code)
	case errors.As(err, &maxBytesErr):
		writeProblem(c, BodyTooLarge)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		writeProblem(c, RequestTimeout)
	default:
		writeProblem(c, InternalError)
	}
}

func writeProblem(c *gin.Context, code ErrorCode) { writeProblemWith(c, code, problemExtension{}) }

func writeProblemWith(c *gin.Context, code ErrorCode, extension problemExtension) {
	spec, ok := problemSpecs[code]
	if !ok {
		code = InternalError
		spec = problemSpecs[code]
	}
	c.Set("error_code", string(code))
	c.Header("Cache-Control", "no-store")
	response, err := json.Marshal(problem{
		Type: spec.typeURI, Title: spec.title, Status: spec.status,
		Detail: spec.detail, Code: code, RequestID: RequestID(c.Request.Context()),
		problemExtension: extension,
	})
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Abort()
	c.Data(spec.status, "application/problem+json; charset=utf-8", response)
}
