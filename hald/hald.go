package hald

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"sync"
)

type HALD struct {
	image.Image
	level int
}

var (
	ErrInvalidDimensions = errors.New("invalid HALD image dimensions")
	ErrNilImage          = errors.New("image is nil")
	ErrDifferentLevels   = errors.New("different HALD levels")
)

type sample struct {
	R, G, B float64
}

func interpolateSamples(s1, s2 sample, t float64) sample {
	return sample{
		R: s1.R + t*(s2.R-s1.R),
		G: s1.G + t*(s2.G-s1.G),
		B: s1.B + t*(s2.B-s1.B),
	}
}

func blendSamples(s1, s2 sample, w1, w2 float64) sample {
	return sample{
		R: s1.R*w1 + s2.R*w2,
		G: s1.G*w1 + s2.G*w2,
		B: s1.B*w1 + s2.B*w2,
	}
}

func colorSample(c color.Color) sample {
	r, g, b, _ := c.RGBA()
	return sample{
		R: float64(r) / 65535.0,
		G: float64(g) / 65535.0,
		B: float64(b) / 65535.0,
	}
}

func min[T int | float64](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func max[T int | float64](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func newHALD(img image.Image) (HALD, error) {
	if img == nil {
		return HALD{}, ErrNilImage
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w != h {
		return HALD{}, ErrInvalidDimensions
	}

	level := int(math.Round(math.Cbrt(float64(w))))
	if level*level*level != w {
		return HALD{}, ErrInvalidDimensions
	}

	return HALD{Image: img, level: level}, nil
}

func (h HALD) Level() int {
	return h.level
}

func (h HALD) sample(r, g, b int) sample {
	cells := h.level * h.level          // samples per axis (N²)
	side := h.level * h.level * h.level // image side (N³)

	idx := b*cells*cells + g*cells + r
	origin := h.Image.Bounds().Min
	return colorSample(h.Image.At(origin.X+idx%side, origin.Y+idx/side))
}

func (h HALD) interpolate(r, g, b float64) sample {
	size := h.level*h.level - 1
	sizeF := float64(size)

	rIdx := max(0.0, min(sizeF, r*sizeF))
	gIdx := max(0.0, min(sizeF, g*sizeF))
	bIdx := max(0.0, min(sizeF, b*sizeF))

	r0, g0, b0 := int(rIdx), int(gIdx), int(bIdx)
	r1, g1, b1 := min(r0+1, size), min(g0+1, size), min(b0+1, size)

	rFrac := rIdx - float64(r0)
	gFrac := gIdx - float64(g0)
	bFrac := bIdx - float64(b0)

	return interpolateSamples(
		interpolateSamples(
			interpolateSamples(h.sample(r0, g0, b0), h.sample(r1, g0, b0), rFrac),
			interpolateSamples(h.sample(r0, g1, b0), h.sample(r1, g1, b0), rFrac),
			gFrac,
		),
		interpolateSamples(
			interpolateSamples(h.sample(r0, g0, b1), h.sample(r1, g0, b1), rFrac),
			interpolateSamples(h.sample(r0, g1, b1), h.sample(r1, g1, b1), rFrac),
			gFrac,
		),
		bFrac,
	)
}

// Interpolate performs trilinear interpolation in the 3D HALD LUT.
func (h HALD) Interpolate(r, g, b float64) (float64, float64, float64) {
	s := h.interpolate(r, g, b)
	return s.R, s.G, s.B
}

func (h HALD) Apply(img image.Image) *image.RGBA {
	return h.ApplyScaled(img, 1.0)
}

func (h HALD) ApplyScaled(img image.Image, intensity float64) *image.RGBA {
	bounds := img.Bounds()
	out := image.NewRGBA(bounds)

	intensity = max(0, min(1, intensity))

	var wg sync.WaitGroup
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		wg.Go(func() {
			h.applyRow(img, out, bounds, y, intensity)
		})
	}
	wg.Wait()
	return out
}

func (h HALD) applyRow(img image.Image, out *image.RGBA, bounds image.Rectangle, y int, intensity float64) {
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		r32, g32, b32, a32 := img.At(x, y).RGBA()

		r := float64(r32) / 65535.0
		g := float64(g32) / 65535.0
		b := float64(b32) / 65535.0

		s := h.interpolate(r, g, b)

		br := max(0, min(1, r+(s.R-r)*intensity))
		bg := max(0, min(1, g+(s.G-g)*intensity))
		bb := max(0, min(1, b+(s.B-b)*intensity))

		out.SetRGBA(x, y, color.RGBA{
			R: uint8(br * 255),
			G: uint8(bg * 255),
			B: uint8(bb * 255),
			A: uint8(a32 / 257),
		})
	}
}

// Blend replaces h with the weighted blend of h and h2 using intensities i1, i2.
func (h *HALD) Blend(h2 HALD, i1, i2 float64) error {
	if h.level != h2.level {
		return ErrDifferentLevels
	}

	total := i1 + i2
	if total == 0 {
		return nil
	}
	w1 := i1 / total
	w2 := i2 / total

	bounds := h.Image.Bounds()
	blended := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			s := blendSamples(
				colorSample(h.Image.At(x, y)),
				colorSample(h2.Image.At(x, y)),
				w1, w2,
			)
			blended.SetRGBA(x, y, color.RGBA{
				R: uint8(s.R * 255),
				G: uint8(s.G * 255),
				B: uint8(s.B * 255),
				A: 255,
			})
		}
	}

	h.Image = blended
	return nil
}

func (h HALD) WriteTo(w io.Writer) (int64, error) {
	return 0, png.Encode(w, h.Image)
}

// Identity creates a neutral HALD of the given level: each input color maps to itself.
func Identity(level int) HALD {
	cells := level * level
	side := level * level * level
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	den := float64(cells - 1)

	for b := range cells {
		for g := range cells {
			for r := range cells {
				idx := b*cells*cells + g*cells + r
				img.SetRGBA(idx%side, idx/side, color.RGBA{
					R: uint8(float64(r) / den * 255),
					G: uint8(float64(g) / den * 255),
					B: uint8(float64(b) / den * 255),
					A: 255,
				})
			}
		}
	}
	return HALD{Image: img, level: level}
}

func Load(r io.Reader) (HALD, error) {
	img, err := png.Decode(r)
	if err != nil {
		return HALD{}, err
	}
	return newHALD(img)
}

func LoadFile(path string) (HALD, error) {
	f, err := os.Open(path)
	if err != nil {
		return HALD{}, err
	}
	defer f.Close()
	return Load(f)
}
