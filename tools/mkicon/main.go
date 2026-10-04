package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"github.com/zolferfigueiredo/weej/internal/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

var appIconSizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
var icoSizes = []int{16, 24, 32, 48, 64, 128, 256}

func main() {
	preview := flag.String("preview", "", "write a visual preview set to this directory instead of the real assets")
	flag.Parse()

	var err error
	if *preview != "" {
		err = writePreview(*preview)
	} else if len(flag.Args()) == 1 {
		err = writeIcons(flag.Args()[0])
	} else {
		fmt.Fprintln(os.Stderr, "usage: mkicon <dir> | mkicon -preview <dir>")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkicon:", err)
		os.Exit(1)
	}
}

func writeIcons(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, size := range appIconSizes {
		path := filepath.Join(dir, fmt.Sprintf("icon-%d.png", size))
		if err := savePNG(path, draw.AppIcon(size)); err != nil {
			return err
		}
	}

	f, err := os.Create(filepath.Join(dir, "weej.ico"))
	if err != nil {
		return err
	}
	defer f.Close()
	return encodeICO(f, icoSizes, func(size int) image.Image { return draw.AppIcon(size) })
}

func writePreview(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, style := range []draw.IconStyle{draw.StyleMixer, draw.StyleDial} {
		for _, px := range []int{16, 32} {
			for _, light := range []bool{true, false} {
				for _, connected := range []bool{true, false} {
					name := fmt.Sprintf("tray-%s-%d-%s-%s.png", style, px,
						boolName(light, "light", "dark"), boolName(connected, "connected", "parked"))
					if err := savePNG(filepath.Join(dir, name), draw.TrayIcon(style, connected, px, light)); err != nil {
						return err
					}
				}
			}
		}
	}

	face, err := loadGoRegular(28)
	if err != nil {
		return err
	}
	defer face.Close()

	for _, dark := range []bool{true, false} {
		ink := color.Color(color.Black)
		if dark {
			ink = color.White
		}
		glyph := draw.Glyph(draw.GlyphSpeaker, 40, ink)
		for _, pct := range []int{0, 37, 100} {
			img := draw.HUD(draw.HUDParams{Scale: 2, Dark: dark, Glyph: glyph, Percent: pct, Face: face})
			name := fmt.Sprintf("hud-%s-%d.png", boolName(dark, "dark", "light"), pct)
			if err := savePNG(filepath.Join(dir, name), img); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadGoRegular(sizePx float64) (font.Face, error) {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{Size: sizePx, DPI: 72, Hinting: font.HintingFull})
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func boolName(v bool, t, f string) string {
	if v {
		return t
	}
	return f
}
