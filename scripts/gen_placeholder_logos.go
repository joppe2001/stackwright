//go:build ignore

// gen_placeholder_logos renders a colored-square + centered monogram PNG for
// every entry in the bundled registry and writes them to a directory the
// user can upload to joppe2001/stackwright-registry/logos/.
//
// Usage:
//
//	go run scripts/gen_placeholder_logos.go -out ../stackwright-registry/logos
//
// Each PNG is 128x128 with the entry's diagram_color filling the background
// and the entry's monogram (first 1-2 letters of words) rendered in white at
// the center via basicfont.Face7x13 scaled up. These are placeholders only —
// contributors are welcome to replace them with real brand marks.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type entry struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	DiagramColor string `json:"diagram_color"`
}

type bundle struct {
	Entries []entry `json:"entries"`
}

func main() {
	outDir := flag.String("out", "./dist/logos", "output directory")
	bundledJSON := flag.String("bundle", "internal/registry/bundled.json", "path to bundled.json")
	flag.Parse()

	data, err := os.ReadFile(*bundledJSON)
	if err != nil {
		log.Fatal(err)
	}
	var b bundle
	if err := json.Unmarshal(data, &b); err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	for _, e := range b.Entries {
		path := filepath.Join(*outDir, e.Slug+".png")
		img := render(e)
		f, err := os.Create(path)
		if err != nil {
			log.Fatalf("%s: %v", path, err)
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			log.Fatalf("%s: %v", path, err)
		}
		_ = f.Close()
		fmt.Println("wrote", path)
	}
}

const size = 128

func render(e entry) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	bg := parseHex(e.DiagramColor)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, bg)
		}
	}
	drawMonogram(img, monogram(e.Name), contrastFG(bg))
	return img
}

// monogram picks the 1-2 letter label for the logo. Same rule as the design
// phase's fallback monogram so visual and standard modes match.
func monogram(name string) string {
	fields := strings.Fields(strings.ReplaceAll(name, "+", " "))
	if len(fields) == 0 {
		return "?"
	}
	if len(fields) == 1 {
		r := []rune(strings.ToUpper(fields[0]))
		if len(r) >= 2 {
			return string(r[:2])
		}
		return string(r[:1])
	}
	var b strings.Builder
	for i := 0; i < 2 && i < len(fields); i++ {
		r := []rune(strings.ToUpper(fields[i]))
		if len(r) > 0 {
			b.WriteRune(r[0])
		}
	}
	return b.String()
}

// drawMonogram writes the label centered on the 128x128 image.
// basicfont is 7x13; we render it into a small tile, then scale it 5x to
// ~35x65 which reads clean on a 128x128 background.
func drawMonogram(dst *image.RGBA, s string, c color.RGBA) {
	tile := image.NewRGBA(image.Rect(0, 0, 7*len(s), 13))
	d := &font.Drawer{
		Dst:  tile,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.Point26_6{X: fixed.I(0), Y: fixed.I(11)},
	}
	d.DrawString(s)

	// Scale tile up by ~5x to fill the logo nicely.
	scale := 5
	dw := tile.Bounds().Dx() * scale
	dh := tile.Bounds().Dy() * scale
	scaled := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.NearestNeighbor.Scale(scaled, scaled.Bounds(), tile, tile.Bounds(), draw.Over, nil)

	// Center-blit onto dst.
	ox := (size - dw) / 2
	oy := (size - dh) / 2
	b := scaled.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			px := scaled.RGBAAt(x, y)
			if px.A == 0 {
				continue
			}
			dst.SetRGBA(ox+x, oy+y, px)
		}
	}
}

// parseHex converts "#rrggbb" to RGBA. Falls back to opaque magenta on malformed input.
func parseHex(hex string) color.RGBA {
	s := strings.TrimPrefix(hex, "#")
	if len(s) != 6 {
		return color.RGBA{R: 255, G: 0, B: 255, A: 255}
	}
	r := hexByte(s[0:2])
	g := hexByte(s[2:4])
	b := hexByte(s[4:6])
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func hexByte(s string) uint8 {
	v := uint8(0)
	for _, c := range s {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= uint8(c - '0')
		case c >= 'a' && c <= 'f':
			v |= uint8(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= uint8(c-'A') + 10
		}
	}
	return v
}

// contrastFG picks black or white text depending on the bg luminance so every
// monogram stays legible regardless of the tech's brand color.
func contrastFG(bg color.RGBA) color.RGBA {
	// sRGB relative luminance. Simple linear formula is accurate enough for
	// the picker's threshold.
	l := 0.2126*float64(bg.R) + 0.7152*float64(bg.G) + 0.0722*float64(bg.B)
	if l > 140 {
		return color.RGBA{R: 11, G: 11, B: 15, A: 255} // canvas bg dark
	}
	return color.RGBA{R: 255, G: 255, B: 255, A: 255}
}
