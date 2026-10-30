package server

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
)

const maxImageBytes = 3 << 20
const maxImageSide = 2000
const maxImagePixels = 4_000_000

var errInvalidImage = errors.New("ungültiges Produktbild")

func RegisterMediaRoutes(router *gin.Engine, repository *catalog.Repository) {
	router.GET("/api/v1/media/products/:id", func(c *gin.Context) {
		id, ok := adminID(c, "id")
		if !ok {
			return
		}
		data, err := repository.GetPublicImage(c.Request.Context(), id)
		if err != nil {
			writeCatalogError(c, err)
			return
		}
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Disposition", `inline; filename="product.jpg"`)
		c.Header("Cache-Control", "public, max-age=300")
		c.Data(http.StatusOK, "image/jpeg", data)
	})
}

func (h *adminHandler) images(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	if _, err := h.repository.GetProduct(c.Request.Context(), id); err != nil {
		adminError(c, err)
		return
	}
	images, err := h.repository.ListImages(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	adminJSON(c, http.StatusOK, gin.H{"data": images})
}

func (h *adminHandler) uploadImage(c *gin.Context) {
	productID, ok := adminID(c, "id")
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		writeProblem(c, InvalidRequest)
		return
	}
	if err := c.Request.ParseMultipartForm(maxImageBodyBytes); err != nil {
		HandleError(c, errForJSON(err))
		return
	}
	if c.Request.MultipartForm == nil {
		writeProblem(c, InvalidRequest)
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	form := c.Request.MultipartForm
	if len(form.File) != 1 || len(form.File["image"]) != 1 || len(form.Value) != 1 || len(form.Value["alt"]) != 1 {
		writeProblem(c, InvalidRequest)
		return
	}
	alt := strings.TrimSpace(form.Value["alt"][0])
	if !validImageAlt(alt) {
		writeProblem(c, InvalidRequest)
		return
	}
	fileHeader := form.File["image"][0]
	if fileHeader.Size <= 0 || fileHeader.Size > maxImageBytes {
		writeProblem(c, BodyTooLarge)
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		HandleError(c, err)
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil {
		HandleError(c, err)
		return
	}
	if len(raw) > maxImageBytes {
		writeProblem(c, BodyTooLarge)
		return
	}
	data, width, height, err := sanitizeProductImage(fileHeader.Header.Get("Content-Type"), raw)
	if err != nil {
		writeProblem(c, InvalidRequest)
		return
	}
	result, err := h.repository.CreateImage(c.Request.Context(), adminActor(c), productID, alt, width, height, data)
	if errors.Is(err, catalog.ErrImageLimit) {
		writeProblem(c, Conflict)
	} else if err != nil {
		adminError(c, err)
	} else {
		adminJSON(c, http.StatusCreated, result)
	}
}

func (h *adminHandler) deleteImage(c *gin.Context) {
	id, ok := adminID(c, "id")
	if !ok {
		return
	}
	if err := h.repository.DeleteImage(c.Request.Context(), adminActor(c), id); err != nil {
		adminError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func validImageAlt(value string) bool {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 200 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// sanitizeProductImage prüft Header, Signatur und Decoder und erzeugt neue
// JPEG-Bytes. Damit gelangen weder Originaldateiname noch Metadaten ins Angebot.
func sanitizeProductImage(declaredType string, raw []byte) ([]byte, int, int, error) {
	mediaType, _, err := mime.ParseMediaType(declaredType)
	if err != nil || (mediaType != "image/jpeg" && mediaType != "image/png") || len(raw) == 0 || len(raw) > maxImageBytes {
		return nil, 0, 0, errInvalidImage
	}
	if http.DetectContentType(raw) != mediaType {
		return nil, 0, 0, errInvalidImage
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || format != strings.TrimPrefix(mediaType, "image/") || config.Width < 1 || config.Height < 1 ||
		config.Width > maxImageSide || config.Height > maxImageSide || config.Width*config.Height > maxImagePixels {
		return nil, 0, 0, errInvalidImage
	}
	source, decodedFormat, err := image.Decode(bytes.NewReader(raw))
	if err != nil || decodedFormat != format {
		return nil, 0, 0, errInvalidImage
	}
	canvas := image.NewRGBA(image.Rect(0, 0, config.Width, config.Height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), source, source.Bounds().Min, draw.Over)
	var output bytes.Buffer
	if err := jpeg.Encode(&output, canvas, &jpeg.Options{Quality: 82}); err != nil || output.Len() > maxImageBytes {
		return nil, 0, 0, errInvalidImage
	}
	return output.Bytes(), config.Width, config.Height, nil
}

var _ = png.Decode // register PNG decoder without accepting other formats
