// Command background renders the family DMG background image.
//
// It draws the graphite backdrop with the "drag to Applications" arrow,
// matching the icon positions declared in macos/dmg-settings.py
// (app at 165,190 and Applications at 495,190 in a 660x400 window).
//
// Usage: go run ./macos/background -out macos/background
// Produces background.png (660x400) and background@2x.png (1320x800).
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const (
	baseW = 660
	baseH = 400
	// arrow geometry in base coordinates (y matches the icon centers)
	arrowY     = 190.0
	arrowX0    = 258.0
	arrowX1    = 402.0
	arrowR     = 6.0
	headLen    = 26.0
	headSpread = 24.0
)

func main() {
	out := flag.String("out", ".", "output directory")
	flag.Parse()

	for _, s := range []struct {
		scale int
		name  string
	}{{1, "background.png"}, {2, "background@2x.png"}} {
		img := render(s.scale)
		path := filepath.Join(*out, s.name)
		f, err := os.Create(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := png.Encode(f, img); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		f.Close()
		fmt.Println("wrote", path)
	}
}

func render(scale int) *image.NRGBA {
	w, h := baseW*scale, baseH*scale
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	fs := float64(scale)

	// arrow as three capsules: shaft plus the two head strokes
	caps := [][4]float64{
		{arrowX0, arrowY, arrowX1, arrowY},
		{arrowX1, arrowY, arrowX1 - headLen, arrowY - headSpread},
		{arrowX1, arrowY, arrowX1 - headLen, arrowY + headSpread},
	}

	cx, cy := float64(baseW)/2, float64(baseH)/2-20
	maxD := math.Hypot(float64(baseW)/2, float64(baseH)/2)

	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			x, y := float64(px)/fs, float64(py)/fs

			// graphite backdrop: subtle radial falloff, family tile tones
			d := math.Hypot(x-cx, y-cy) / maxD
			t := d * d // ease toward the edges
			r := lerp(0x2e, 0x1f, t)
			g := lerp(0x2e, 0x1f, t)
			b := lerp(0x33, 0x23, t)

			// arrow coverage (antialiased)
			a := 0.0
			for _, c := range caps {
				a = math.Max(a, capsule(x, y, c[0], c[1], c[2], c[3], arrowR, fs))
			}
			if a > 0 {
				const ar, ag, ab = 0x8c, 0x8c, 0x92
				r = r*(1-a) + ar*a
				g = g*(1-a) + ag*a
				b = b*(1-a) + ab*a
			}

			img.SetNRGBA(px, py, color.NRGBA{uint8(r), uint8(g), uint8(b), 0xff})
		}
	}
	return img
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// capsule returns antialiased coverage of a rounded stroke from (ax,ay) to
// (bx,by) with radius r, sampled at point (x,y); fs is the render scale.
func capsule(x, y, ax, ay, bx, by, r, fs float64) float64 {
	vx, vy := bx-ax, by-ay
	wx, wy := x-ax, y-ay
	t := (wx*vx + wy*vy) / (vx*vx + vy*vy)
	t = math.Max(0, math.Min(1, t))
	d := math.Hypot(x-(ax+t*vx), y-(ay+t*vy))
	edge := 0.75 / fs
	return math.Max(0, math.Min(1, (r-d)/edge/2+0.5))
}
