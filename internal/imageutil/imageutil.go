package imageutil

import (
	"encoding/binary"
	"fmt"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
)

type Metadata struct {
	Format      string
	Width       int
	Height      int
	AspectRatio string
	HasAlpha    bool
}

func GetMetadata(path string) *Metadata {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var header [12]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return nil
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil
	}

	var meta *Metadata

	switch {
	case isJPEG(header[:]):
		meta = decodeJPEG(f)
	case isPNG(header[:]):
		meta = decodePNG(f)
	case isGIF(header[:]):
		meta = decodeGIF(f)
	case isWebP(header[:]):
		meta = decodeWebP(f)
	case isICO(header[:]):
		meta = decodeICO(f)
	default:
		return nil
	}

	if meta == nil {
		return nil
	}

	meta.AspectRatio = formatAspectRatio(meta.Width, meta.Height)
	return meta
}

func isJPEG(b []byte) bool {
	return len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8
}

func isPNG(b []byte) bool {
	return len(b) >= 4 && b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G'
}

func isGIF(b []byte) bool {
	return len(b) >= 3 && b[0] == 'G' && b[1] == 'I' && b[2] == 'F'
}

func isWebP(b []byte) bool {
	return len(b) >= 4 && b[0] == 'R' && b[1] == 'I' && b[2] == 'F' && b[3] == 'F'
}

func isICO(b []byte) bool {
	return len(b) >= 4 && b[0] == 0x00 && b[1] == 0x00 && b[2] == 0x01 && b[3] == 0x00
}

func decodeJPEG(f *os.File) *Metadata {
	cfg, err := jpeg.DecodeConfig(f)
	if err != nil {
		return nil
	}
	return &Metadata{
		Format: "JPEG",
		Width:  cfg.Width,
		Height: cfg.Height,
	}
}

func decodePNG(f *os.File) *Metadata {
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return nil
	}

	hasAlpha := readPNGAlpha(f)

	return &Metadata{
		Format:   "PNG",
		Width:    cfg.Width,
		Height:   cfg.Height,
		HasAlpha: hasAlpha,
	}
}

func readPNGAlpha(f *os.File) bool {
	if _, err := f.Seek(25, io.SeekStart); err != nil {
		return false
	}
	var colorType [1]byte
	if _, err := io.ReadFull(f, colorType[:]); err != nil {
		return false
	}
	return colorType[0] == 4 || colorType[0] == 6
}

func decodeGIF(f *os.File) *Metadata {
	cfg, err := gif.DecodeConfig(f)
	if err != nil {
		return nil
	}
	return &Metadata{
		Format: "GIF",
		Width:  cfg.Width,
		Height: cfg.Height,
	}
}

func decodeWebP(f *os.File) *Metadata {
	buf := make([]byte, 30)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil
	}

	if string(buf[8:12]) != "WEBP" || string(buf[12:16]) != "VP8X" {
		return nil
	}

	flags := buf[20]
	width := int(u24LE(buf[24:27])) + 1
	height := int(u24LE(buf[27:30])) + 1

	return &Metadata{
		Format:   "WebP",
		Width:    width,
		Height:   height,
		HasAlpha: flags&0x10 != 0,
	}
}

func decodeICO(f *os.File) *Metadata {
	var header [6]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return nil
	}

	count := int(binary.LittleEndian.Uint16(header[4:6]))
	if count == 0 {
		return nil
	}

	var entry [16]byte
	if _, err := io.ReadFull(f, entry[:]); err != nil {
		return nil
	}

	width := int(entry[0])
	height := int(entry[1])
	if width == 0 {
		width = 256
	}
	if height == 0 {
		height = 256
	}

	return &Metadata{
		Format: "ICO",
		Width:  width,
		Height: height,
	}
}

func u24LE(b []byte) uint32 {
	if len(b) < 3 {
		return 0
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
}

func formatAspectRatio(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	g := gcd(w, h)
	return fmt.Sprintf("%d:%d", w/g, h/g)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
