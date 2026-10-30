package server

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"testing"
)

func TestSanitizeProductImage(t *testing.T) {
	imageData := image.NewRGBA(image.Rect(0, 0, 2, 2))
	imageData.Set(0, 0, color.RGBA{R: 220, A: 255})
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, imageData); err != nil {
		t.Fatal(err)
	}
	withPayload := append(append([]byte{}, pngData.Bytes()...), []byte("<script>alert(1)</script>")...)
	clean, width, height, err := sanitizeProductImage("image/png", withPayload)
	if err != nil || width != 2 || height != 2 {
		t.Fatalf("PNG-Bereinigung: %d x %d, %v", width, height, err)
	}
	if http.DetectContentType(clean) != "image/jpeg" || bytes.Contains(clean, []byte("<script>")) {
		t.Fatal("Ausgabe ist kein neu kodiertes JPEG")
	}
	if _, err := jpeg.Decode(bytes.NewReader(clean)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		mime string
		data []byte
	}{
		{"falscher MIME-Typ", "image/jpeg", pngData.Bytes()},
		{"ausführbarer Inhalt", "image/png", []byte("<script>alert(1)</script>")},
		{"zu groß", "image/png", make([]byte, maxImageBytes+1)},
		{"leere Datei", "image/png", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, _, err := sanitizeProductImage(test.mime, test.data); err == nil {
				t.Fatal("Ungültiges Bild akzeptiert")
			}
		})
	}
	large := image.NewRGBA(image.Rect(0, 0, maxImageSide+1, 1))
	var largeData bytes.Buffer
	if err := png.Encode(&largeData, large); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := sanitizeProductImage("image/png", largeData.Bytes()); err == nil {
		t.Fatal("Überdimensioniertes Bild akzeptiert")
	}
}
