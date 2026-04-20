package qrutil

import (
	"bytes"
	"image/png"
	"testing"
)

func TestGeneratePNG(t *testing.T) {
	content := "tg://proxy?server=1.2.3.4&port=443&secret=aaaabbbbccccddddaaaabbbbccccdddd"
	data, err := GeneratePNG(content, 256)
	if err != nil {
		t.Fatalf("GeneratePNG() returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GeneratePNG() returned empty bytes")
	}

	// Verify it's a valid PNG
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("GeneratePNG() output is not a valid PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 256 || bounds.Dy() != 256 {
		t.Errorf("GeneratePNG(256) image size = %dx%d, want 256x256", bounds.Dx(), bounds.Dy())
	}
}

func TestGeneratePNGLargeSize(t *testing.T) {
	data, err := GeneratePNG("hello world", 512)
	if err != nil {
		t.Fatalf("GeneratePNG(512) returned error: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("GeneratePNG(512) output is not a valid PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 512 || bounds.Dy() != 512 {
		t.Errorf("GeneratePNG(512) image size = %dx%d, want 512x512", bounds.Dx(), bounds.Dy())
	}
}

func TestGeneratePNGDefaultSize(t *testing.T) {
	// size <= 0 should use DefaultSize (256)
	data, err := GeneratePNG("test content", 0)
	if err != nil {
		t.Fatalf("GeneratePNG(0) returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GeneratePNG(0) returned empty bytes")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("GeneratePNG(0) output is not a valid PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != DefaultSize || bounds.Dy() != DefaultSize {
		t.Errorf("GeneratePNG(0) image size = %dx%d, want %dx%d (DefaultSize)",
			bounds.Dx(), bounds.Dy(), DefaultSize, DefaultSize)
	}
}

func TestGeneratePNGNegativeSize(t *testing.T) {
	// Negative size should fall back to DefaultSize
	data, err := GeneratePNG("test content", -10)
	if err != nil {
		t.Fatalf("GeneratePNG(-10) returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GeneratePNG(-10) returned empty bytes")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("GeneratePNG(-10) output is not a valid PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != DefaultSize || bounds.Dy() != DefaultSize {
		t.Errorf("GeneratePNG(-10) image size = %dx%d, want %dx%d (DefaultSize)",
			bounds.Dx(), bounds.Dy(), DefaultSize, DefaultSize)
	}
}

func TestGenerateBytes(t *testing.T) {
	content := "https://t.me/proxy?server=1.2.3.4&port=443&secret=test"
	data, err := GenerateBytes(content)
	if err != nil {
		t.Fatalf("GenerateBytes() returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GenerateBytes() returned empty bytes")
	}
	// Verify it's a valid PNG
	_, err = png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("GenerateBytes() output is not a valid PNG: %v", err)
	}
}

func TestGeneratePNGLongContent(t *testing.T) {
	// Use a realistic proxy link (longer than a simple word but within QR limits)
	content := "tg://proxy?server=192.168.1.100&port=8443&secret=aaaabbbbccccddddaaaabbbbccccdddd&underlay_dd=aaaabbbbccccddddaaaabbbbccccdddd"
	data, err := GeneratePNG(content, 256)
	if err != nil {
		t.Fatalf("GeneratePNG() with realistic long content returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GeneratePNG() with long content returned empty bytes")
	}
	// Verify it's a valid PNG
	_, err = png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("GeneratePNG() long content output is not a valid PNG: %v", err)
	}
}

func TestGeneratePNGMinimalSize(t *testing.T) {
	data, err := GeneratePNG("a", 64)
	if err != nil {
		t.Fatalf("GeneratePNG() with size=64 returned error: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("GeneratePNG(64) output is not a valid PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Errorf("GeneratePNG(64) image size = %dx%d, want 64x64", bounds.Dx(), bounds.Dy())
	}
}
