package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
)

type AuthConfig struct {
	PublicOrigin         string
	SecureCookie         bool
	TrustedProxyClientIP bool
	RegistrationNotifier interface {
		SendWelcome(ctx context.Context, recipient string) error
	}
	Logger *slog.Logger
}

type AuthHandler struct {
	service *identity.Service
	config  AuthConfig
}

const authSessionKey = "nordwinds.auth.session"

func RegisterAuthRoutes(router *gin.Engine, service *identity.Service, config AuthConfig) *AuthHandler {
	handler := &AuthHandler{service: service, config: config}
	auth := router.Group("/api/v1/auth")
	auth.POST("/register", handler.register)
	auth.POST("/login", handler.login)
	auth.GET("/session", handler.RequireSession(), handler.session)
	auth.POST("/logout", handler.RequireSession(), handler.RequireCSRF(), handler.logout)
	router.GET("/api/v1/customer/me", handler.RequireSession(), handler.RequireRole(identity.CustomerRole), handler.customer)
	return handler
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) register(c *gin.Context) {
	if !h.safeOrigin(c) {
		writeProblem(c, Forbidden)
		return
	}
	var request credentialsRequest
	if err := readJSON(c, &request); err != nil {
		HandleError(c, err)
		return
	}
	customer, err := h.service.Register(c.Request.Context(), request.Email, request.Password, h.clientSource(c))
	switch {
	case errors.Is(err, identity.ErrInvalidInput):
		writeProblem(c, InvalidRequest)
	case errors.Is(err, identity.ErrDuplicate):
		writeProblem(c, Conflict)
	case errors.Is(err, identity.ErrRateLimited):
		c.Header("Retry-After", "900")
		writeProblem(c, RateLimited)
	case err != nil:
		HandleError(c, err)
	default:
		if h.config.RegistrationNotifier != nil {
			if notifyErr := h.config.RegistrationNotifier.SendWelcome(c.Request.Context(), customer.Email); notifyErr != nil && h.config.Logger != nil {
				h.config.Logger.Error("Registrierungs-E-Mail konnte nicht gesendet werden", "event", "registration.mail.failed")
			}
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, gin.H{"customer": customer})
	}
}

func (h *AuthHandler) login(c *gin.Context) {
	if !h.safeOrigin(c) {
		writeProblem(c, Forbidden)
		return
	}
	var request credentialsRequest
	if err := readJSON(c, &request); err != nil {
		HandleError(c, err)
		return
	}
	oldToken := h.cookieToken(c)
	customer, token, session, err := h.service.Login(c.Request.Context(), request.Email, request.Password, h.clientSource(c), oldToken)
	switch {
	case errors.Is(err, identity.ErrInvalidCredentials):
		writeProblem(c, Unauthenticated)
	case errors.Is(err, identity.ErrRateLimited):
		c.Header("Retry-After", "900")
		writeProblem(c, RateLimited)
	case err != nil:
		HandleError(c, err)
	default:
		h.setCookie(c, token, false)
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"customer": customer, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
	}
}

func (h *AuthHandler) session(c *gin.Context) {
	session := CurrentSession(c)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"customer": session.Customer, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
}

func (h *AuthHandler) customer(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"customer": CurrentSession(c).Customer})
}

func (h *AuthHandler) logout(c *gin.Context) {
	if err := h.service.Logout(c.Request.Context(), h.cookieToken(c)); err != nil {
		HandleError(c, err)
		return
	}
	h.setCookie(c, "", true)
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) RequireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := h.service.Authenticate(c.Request.Context(), h.cookieToken(c))
		if errors.Is(err, identity.ErrUnauthenticated) {
			writeProblem(c, Unauthenticated)
			return
		}
		if err != nil {
			HandleError(c, err)
			return
		}
		c.Set(authSessionKey, session)
		c.Next()
	}
}

func (h *AuthHandler) RequireRole(role identity.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		if CurrentSession(c).Customer.Role != role {
			writeProblem(c, Forbidden)
			return
		}
		c.Next()
	}
}

func (h *AuthHandler) RequireCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.safeOrigin(c) {
			writeProblem(c, Forbidden)
			return
		}
		token := c.GetHeader("X-CSRF-Token")
		want := CurrentSession(c).CSRFToken
		if len(token) != 43 || subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
			writeProblem(c, Forbidden)
			return
		}
		c.Next()
	}
}

func CurrentSession(c *gin.Context) identity.Session {
	value, _ := c.Get(authSessionKey)
	session, _ := value.(identity.Session)
	return session
}

func (h *AuthHandler) cookieName() string {
	if h.config.SecureCookie {
		return "__Host-nordwinds_session"
	}
	return "nordwinds_session"
}

// Nur der isoliert angeschlossene Edge-Proxy darf die Herkunfts-IP liefern.
// Ohne diese explizite Konfiguration zählt ausschließlich die TCP-Gegenstelle.
func (h *AuthHandler) clientSource(c *gin.Context) string {
	source, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		source = c.Request.RemoteAddr
	}
	if h.config.TrustedProxyClientIP {
		forwarded := c.GetHeader("X-Nordwinds-Client-IP")
		if net.ParseIP(forwarded) != nil {
			return forwarded
		}
	}
	return source
}

func (h *AuthHandler) cookieToken(c *gin.Context) string {
	token, _ := c.Cookie(h.cookieName())
	return token
}

func (h *AuthHandler) setCookie(c *gin.Context, token string, clear bool) {
	cookie := &http.Cookie{
		Name: h.cookieName(), Value: token, Path: "/", HttpOnly: true,
		Secure: h.config.SecureCookie, SameSite: http.SameSiteStrictMode,
	}
	if clear {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(0, 0)
	}
	http.SetCookie(c.Writer, cookie)
}

func (h *AuthHandler) safeOrigin(c *gin.Context) bool {
	if strings.EqualFold(c.GetHeader("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	expected := h.config.PublicOrigin
	if expected == "" {
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		expected = scheme + "://" + c.Request.Host
	}
	for _, raw := range []string{c.GetHeader("Origin"), c.GetHeader("Referer")} {
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return false
		}
		origin := parsed.Scheme + "://" + parsed.Host
		if !strings.EqualFold(origin, expected) && !h.localOriginAlias(c, expected, origin) {
			return false
		}
	}
	return true
}

// Im HTTP-Entwicklungsbetrieb darf derselbe lokale Shop-Port sowohl über
// localhost als auch über 127.0.0.1 geöffnet werden. Der Request-Host muss
// weiterhin exakt zum gesendeten Origin passen; HTTPS bleibt strikt.
func (h *AuthHandler) localOriginAlias(c *gin.Context, expected, origin string) bool {
	if h.config.SecureCookie {
		return false
	}
	configured, configuredErr := url.Parse(expected)
	requested, requestedErr := url.Parse(origin)
	return configuredErr == nil && requestedErr == nil &&
		configured.Scheme == "http" && requested.Scheme == "http" &&
		configured.Port() == requested.Port() &&
		localHost(configured.Hostname()) && localHost(requested.Hostname()) &&
		strings.EqualFold(c.Request.Host, requested.Host)
}

func localHost(host string) bool {
	return strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1"
}

func readJSON(c *gin.Context, target any) error {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return &APIError{Code: InvalidRequest}
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errForJSON(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return errForJSON(err)
		}
		return &APIError{Code: InvalidRequest}
	}
	return nil
}

func errForJSON(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return err
	}
	return &APIError{Code: InvalidRequest}
}
