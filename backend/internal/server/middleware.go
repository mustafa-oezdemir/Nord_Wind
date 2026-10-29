package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxBodyBytes    = 1 << 20
	maxTargetBytes  = 2048
	requestDeadline = 5 * time.Second
)

type requestIDKey struct{}

// RequestID liest die serverseitig erzeugte Kennung aus dem Request-Kontext.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func assignRequestID(c *gin.Context) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	id := hex.EncodeToString(raw[:])
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDKey{}, id))
	c.Header("X-Request-ID", id)
	c.Next()
}

func accessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		level := slog.LevelInfo
		if c.Writer.Status() >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		attrs := []slog.Attr{
			slog.String("event", "http.request"),
			slog.String("request_id", RequestID(c.Request.Context())),
			slog.String("http_method", c.Request.Method),
			slog.String("route", route),
			slog.Int("status_code", c.Writer.Status()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		}
		if code := c.GetString("error_code"); code != "" {
			attrs = append(attrs, slog.String("error_code", code))
		}
		logger.LogAttrs(c.Request.Context(), level, "HTTP-Anfrage", attrs...)
	}
}

func recoverProblem(c *gin.Context) {
	defer func() {
		if recover() != nil {
			if c.Writer.Written() {
				c.Set("error_code", string(InternalError))
				c.Abort()
				return
			}
			writeProblem(c, InternalError)
		}
	}()
	c.Next()
}

func limitInput(c *gin.Context) {
	if len(c.Request.RequestURI) > maxTargetBytes {
		writeProblem(c, URITooLong)
		return
	}
	if c.Request.ContentLength > maxBodyBytes {
		writeProblem(c, BodyTooLarge)
		return
	}
	if c.Request.Body != nil {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	}
	c.Next()
}

func withDeadline(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestDeadline)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	c.Next()
	if ctx.Err() == context.DeadlineExceeded && !c.Writer.Written() {
		writeProblem(c, RequestTimeout)
	}
}
