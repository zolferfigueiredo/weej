// Command mockglyphs writes the glyphs Settings shows, in white for dark and black for light as
// the app draws them, for the browser preview in bridge.js, which has no Go to draw them:
//
//	go run ./tools/mockglyphs internal/ui/web/mockglyphs.json
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/color"
	"image/png"
	"os"

	"github.com/zolferfigueiredo/weej/internal/draw"
)

var glyphs = map[string]draw.GlyphKind{
	"speaker":       draw.GlyphSpeaker,
	"speakerMuted":  draw.GlyphSpeakerMuted,
	"mic":           draw.GlyphMic,
	"sun":           draw.GlyphSun,
	"contrast":      draw.GlyphContrast,
	"moon":          draw.GlyphMoon,
	"keyboard":      draw.GlyphKeyboard,
	"zoom":          draw.GlyphZoom,
	"app":           draw.GlyphApp,
	"play":          draw.GlyphPlay,
	"pause":         draw.GlyphPause,
	"playPause":     draw.GlyphPlayPause,
	"stop":          draw.GlyphStop,
	"previousTrack": draw.GlyphPreviousTrack,
	"nextTrack":     draw.GlyphNextTrack,
	"volumeUp":      draw.GlyphVolumeUp,
	"volumeDown":    draw.GlyphVolumeDown,
	"micMuted":      draw.GlyphMicMuted,
	"closeApp":      draw.GlyphCloseApp,
	"globe":         draw.GlyphGlobe,
	"screen":        draw.GlyphScreen,
	"lock":          draw.GlyphLock,
	"power":         draw.GlyphPower,
	"arrowLeft":     draw.GlyphArrowLeft,
	"arrowRight":    draw.GlyphArrowRight,
	"list":          draw.GlyphList,
	"gear":          draw.GlyphGear,
	"bulb":          draw.GlyphBulb,
	"bulbOn":        draw.GlyphBulbOn,
	"bulbOff":       draw.GlyphBulbOff,
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mockglyphs <out.json>")
		os.Exit(2)
	}
	if err := write(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func write(path string) error {
	inks := map[string]color.Color{"dark": color.White, "light": color.Black}
	out := map[string]map[string]string{}
	for theme, ink := range inks {
		out[theme] = map[string]string{}
		for name, kind := range glyphs {
			var buf bytes.Buffer
			if err := png.Encode(&buf, draw.Glyph(kind, 32, ink)); err != nil {
				return err
			}
			out[theme][name] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
		}
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
