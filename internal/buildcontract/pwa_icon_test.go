package buildcontract

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestMaskablePWAIconHasAnOpaqueBackgroundAndVisibleMark(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	path := filepath.Join(repository, "web", "public", "icon-maskable-512.png")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open maskable PWA icon: %v", err)
	}
	defer file.Close()
	icon, format, err := image.Decode(file)
	if err != nil {
		t.Fatalf("decode maskable PWA icon: %v", err)
	}
	if format != "png" {
		t.Fatalf("maskable PWA icon format = %q, want png", format)
	}
	if bounds := icon.Bounds(); bounds.Dx() != 512 || bounds.Dy() != 512 {
		t.Fatalf("maskable PWA icon bounds = %v, want 512x512", bounds)
	}

	for _, point := range []image.Point{{0, 0}, {511, 0}, {0, 511}, {511, 511}} {
		red, green, blue, alpha := icon.At(point.X, point.Y).RGBA()
		if alpha != 0xffff || red > 0x2000 || green > 0x2800 || blue > 0x4000 {
			t.Errorf("maskable PWA icon corner %v = rgba(%04x,%04x,%04x,%04x), want an opaque dark background", point, red, green, blue, alpha)
		}
	}

	visiblePixels := 0
	for y := 96; y < 416; y++ {
		for x := 96; x < 416; x++ {
			red, green, blue, alpha := icon.At(x, y).RGBA()
			if alpha == 0xffff && blue > 0x8000 && (red > 0x2800 || green > 0x5000) {
				visiblePixels++
			}
		}
	}
	if visiblePixels < 5_000 {
		t.Fatalf("maskable PWA icon has %d visible mark pixels, want at least 5000", visiblePixels)
	}
}
