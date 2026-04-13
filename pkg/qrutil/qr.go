package qrutil

import (
	"bytes"
	"fmt"
	"image/png"

	qrcode "github.com/skip2/go-qrcode"
)

// DefaultSize is the default QR code image size in pixels.
const DefaultSize = 256

// GeneratePNG generates a QR code as PNG bytes.
func GeneratePNG(content string, size int) ([]byte, error) {
	if size <= 0 {
		size = DefaultSize
	}

	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("create qr: %w", err)
	}
	qr.DisableBorder = false

	img := qr.Image(size)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// GenerateBytes is a shorthand that returns raw PNG bytes for a proxy link.
func GenerateBytes(content string) ([]byte, error) {
	return GeneratePNG(content, DefaultSize)
}
