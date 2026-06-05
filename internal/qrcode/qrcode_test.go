package qrcode

import (
	"bytes"
	"image/png"
	"testing"
)

func TestPNG(t *testing.T) {
	data, err := PNG("itms-services://?action=download-manifest&url=https%3A%2F%2Fexample.com%2Fmanifest.plist", 4, 4)
	if err != nil {
		t.Fatalf("PNG() error = %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png.Decode() error = %v", err)
	}
	if img.Bounds().Dx() != img.Bounds().Dy() {
		t.Fatalf("png should be square, got %v", img.Bounds())
	}
	if img.Bounds().Dx() == 0 {
		t.Fatal("png should not be empty")
	}
}

func TestMatrixRejectsLongContent(t *testing.T) {
	_, err := Matrix(bytes.Repeat([]byte("x"), maxBytes+1))
	if err == nil {
		t.Fatal("Matrix() expected error")
	}
}
