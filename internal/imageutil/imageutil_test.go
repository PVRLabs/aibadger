package imageutil

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func createTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func createJPEG(t *testing.T, w, h int) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test*.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func createPNG(t *testing.T, w, h int, withAlpha bool) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var img image.Image
	if withAlpha {
		img = image.NewNRGBA(image.Rect(0, 0, w, h))
	} else {
		img = image.NewGray(image.Rect(0, 0, w, h))
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func createGIF(t *testing.T, w, h int) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test*.gif")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img := image.NewPaletted(image.Rect(0, 0, w, h), []color.Color{
		color.Transparent,
		color.Black,
		color.White,
	})
	if err := gif.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func createMinimalWebP(t *testing.T, w, h int, withAlpha bool) string {
	t.Helper()

	width := w - 1
	height := h - 1

	var vp8xFlags byte
	if withAlpha {
		vp8xFlags = 0x10
	}

	payload := make([]byte, 30)
	copy(payload, []byte("RIFF"))
	payload[4] = 22 // file size - 8 (just the VP8X chunk)
	copy(payload[8:12], "WEBP")
	copy(payload[12:16], "VP8X")
	payload[16] = 10 // chunk size
	payload[17] = 0
	payload[18] = 0
	payload[19] = 0
	payload[20] = vp8xFlags
	payload[21] = 0
	payload[22] = 0
	payload[23] = 0
	payload[24] = byte(width)
	payload[25] = byte(width >> 8)
	payload[26] = byte(width >> 16)
	payload[27] = byte(height)
	payload[28] = byte(height >> 8)
	payload[29] = byte(height >> 16)

	return createTempFile(t, "test.webp", payload)
}

func createMinimalICO(t *testing.T, w, h int, bpp int) string {
	t.Helper()

	entryWidth := byte(w)
	if w >= 256 {
		entryWidth = 0
	}
	entryHeight := byte(h)
	if h >= 256 {
		entryHeight = 0
	}

	var buf []byte
	buf = append(buf, 0x00, 0x00)
	buf = append(buf, 0x01, 0x00) // type: ICO
	buf = append(buf, 0x01, 0x00) // count: 1
	buf = append(buf, entryWidth)
	buf = append(buf, entryHeight)
	buf = append(buf, 0x00)                   // colors
	buf = append(buf, 0x00)                   // reserved
	buf = append(buf, 0x01, 0x00)             // color planes
	buf = append(buf, byte(bpp), 0x00)        // bits per pixel
	buf = append(buf, 0x00, 0x00, 0x00, 0x00) // image size
	buf = append(buf, 22, 0x00, 0x00, 0x00)   // image offset (past header + entry)

	return createTempFile(t, "test.ico", buf)
}

func TestGetMetadataJPEG(t *testing.T) {
	path := createJPEG(t, 100, 50)
	meta := GetMetadata(path)
	if meta == nil {
		t.Fatal("GetMetadata returned nil")
	}
	if meta.Format != "JPEG" {
		t.Errorf("Format = %q, want JPEG", meta.Format)
	}
	if meta.Width != 100 {
		t.Errorf("Width = %d, want 100", meta.Width)
	}
	if meta.Height != 50 {
		t.Errorf("Height = %d, want 50", meta.Height)
	}
	if meta.AspectRatio != "2:1" {
		t.Errorf("AspectRatio = %q, want 2:1", meta.AspectRatio)
	}
	if meta.HasAlpha {
		t.Error("HasAlpha = true, want false for JPEG")
	}
}

func TestGetMetadataPNGWithAlpha(t *testing.T) {
	path := createPNG(t, 200, 100, true)
	meta := GetMetadata(path)
	if meta == nil {
		t.Fatal("GetMetadata returned nil")
	}
	if meta.Format != "PNG" {
		t.Errorf("Format = %q, want PNG", meta.Format)
	}
	if meta.Width != 200 {
		t.Errorf("Width = %d, want 200", meta.Width)
	}
	if meta.Height != 100 {
		t.Errorf("Height = %d, want 100", meta.Height)
	}
	if !meta.HasAlpha {
		t.Error("HasAlpha = false, want true for NRGBA PNG")
	}
}

func TestGetMetadataPNGWithoutAlpha(t *testing.T) {
	path := createPNG(t, 150, 75, false)
	meta := GetMetadata(path)
	if meta == nil {
		t.Fatal("GetMetadata returned nil")
	}
	if meta.Format != "PNG" {
		t.Errorf("Format = %q, want PNG", meta.Format)
	}
	if meta.Width != 150 {
		t.Errorf("Width = %d, want 150", meta.Width)
	}
	if meta.Height != 75 {
		t.Errorf("Height = %d, want 75", meta.Height)
	}
	if meta.HasAlpha {
		t.Error("HasAlpha = true, want false for grayscale PNG")
	}
}

func TestGetMetadataGIF(t *testing.T) {
	path := createGIF(t, 300, 200)
	meta := GetMetadata(path)
	if meta == nil {
		t.Fatal("GetMetadata returned nil")
	}
	if meta.Format != "GIF" {
		t.Errorf("Format = %q, want GIF", meta.Format)
	}
	if meta.Width != 300 {
		t.Errorf("Width = %d, want 300", meta.Width)
	}
	if meta.Height != 200 {
		t.Errorf("Height = %d, want 200", meta.Height)
	}
}

func TestGetMetadataWebPWithoutAlpha(t *testing.T) {
	path := createMinimalWebP(t, 120, 63, false)
	meta := GetMetadata(path)
	if meta == nil {
		t.Fatal("GetMetadata returned nil")
	}
	if meta.Format != "WebP" {
		t.Errorf("Format = %q, want WebP", meta.Format)
	}
	if meta.Width != 120 {
		t.Errorf("Width = %d, want 120", meta.Width)
	}
	if meta.Height != 63 {
		t.Errorf("Height = %d, want 63", meta.Height)
	}
	if meta.HasAlpha {
		t.Error("HasAlpha = true, want false")
	}
}

func TestGetMetadataWebPWithAlpha(t *testing.T) {
	path := createMinimalWebP(t, 120, 63, true)
	meta := GetMetadata(path)
	if meta == nil {
		t.Fatal("GetMetadata returned nil")
	}
	if meta.Format != "WebP" {
		t.Errorf("Format = %q, want WebP", meta.Format)
	}
	if !meta.HasAlpha {
		t.Error("HasAlpha = false, want true")
	}
}

func TestGetMetadataICO(t *testing.T) {
	path := createMinimalICO(t, 32, 32, 32)
	meta := GetMetadata(path)
	if meta == nil {
		t.Fatal("GetMetadata returned nil")
	}
	if meta.Format != "ICO" {
		t.Errorf("Format = %q, want ICO", meta.Format)
	}
	if meta.Width != 32 {
		t.Errorf("Width = %d, want 32", meta.Width)
	}
	if meta.Height != 32 {
		t.Errorf("Height = %d, want 32", meta.Height)
	}
}

func TestGetMetadataICOLarge(t *testing.T) {
	path := createMinimalICO(t, 256, 256, 24)
	meta := GetMetadata(path)
	if meta == nil {
		t.Fatal("GetMetadata returned nil")
	}
	if meta.Width != 256 {
		t.Errorf("Width = %d, want 256", meta.Width)
	}
	if meta.Height != 256 {
		t.Errorf("Height = %d, want 256", meta.Height)
	}
}

func TestGetMetadataRealHeroPNG(t *testing.T) {
	// Use the existing hero.png asset in the project
	path := filepath.Join("..", "..", "assets", "hero.png")
	if _, err := os.Stat(path); err != nil {
		t.Skip("hero.png not found:", err)
	}
	meta := GetMetadata(path)
	if meta == nil {
		t.Fatal("GetMetadata returned nil for real hero.png")
	}
	if meta.Format != "PNG" {
		t.Errorf("Format = %q, want PNG", meta.Format)
	}
	if meta.Width != 800 {
		t.Errorf("Width = %d, want 800", meta.Width)
	}
	if meta.Height != 451 {
		t.Errorf("Height = %d, want 451", meta.Height)
	}
	if meta.HasAlpha {
		t.Error("HasAlpha = true, want false for RGB PNG")
	}
	if meta.AspectRatio != "800:451" {
		t.Errorf("AspectRatio = %q, want 800:451", meta.AspectRatio)
	}
}

func TestGetMetadataNonExistentFile(t *testing.T) {
	meta := GetMetadata("/nonexistent/path/image.png")
	if meta != nil {
		t.Fatal("GetMetadata should return nil for non-existent file")
	}
}

func TestGetMetadataNonImageFile(t *testing.T) {
	path := createTempFile(t, "not_an_image.bin", []byte{0x00, 0x01, 0x02, 0x03})
	meta := GetMetadata(path)
	if meta != nil {
		t.Fatal("GetMetadata should return nil for binary file")
	}
}

func TestGetMetadataTextFile(t *testing.T) {
	path := createTempFile(t, "text.txt", []byte("hello world"))
	meta := GetMetadata(path)
	if meta != nil {
		t.Fatal("GetMetadata should return nil for text file")
	}
}

func TestGetMetadataCorruptedPNG(t *testing.T) {
	path := createTempFile(t, "corrupted.png", []byte("not a valid png at all"))
	meta := GetMetadata(path)
	if meta != nil {
		t.Fatal("GetMetadata should return nil for corrupted PNG")
	}
}

func TestFormatAspectRatio(t *testing.T) {
	tests := []struct {
		w, h int
		want string
	}{
		{1920, 1080, "16:9"},
		{1200, 630, "40:21"},
		{640, 480, "4:3"},
		{1, 1, "1:1"},
		{0, 100, ""},
		{100, 0, ""},
		{-1, 100, ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%dx%d", tt.w, tt.h), func(t *testing.T) {
			got := formatAspectRatio(tt.w, tt.h)
			if got != tt.want {
				t.Errorf("formatAspectRatio(%d, %d) = %q, want %q", tt.w, tt.h, got, tt.want)
			}
		})
	}
}

func TestGCD(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{12, 8, 4},
		{16, 9, 1},
		{100, 75, 25},
		{7, 3, 1},
		{0, 5, 5},
		{5, 0, 5},
		{0, 0, 0},
	}
	for _, tt := range tests {
		got := gcd(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("gcd(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
