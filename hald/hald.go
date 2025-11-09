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

// newHALD creates a HALD from an image after validating dimensions
func newHALD(img image.Image) (HALD, error) {
	if img == nil {
		return HALD{}, ErrNilImage
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w != h {
		return HALD{}, ErrInvalidDimensions
	}

	// level = round(cuberoot(width))
	levelF := math.Round(math.Cbrt(float64(w)))
	level := int(levelF)
	if level*level*level != w {
		return HALD{}, ErrInvalidDimensions
	}

	return HALD{Image: img, level: level}, nil
}

// sample retrieves the color at the given 3D cube coordinates
// r, g, b should be in range [0, level-1]
func (h HALD) sample(r, g, b int) color.Color {
	N := h.level
	cube := N * N
	size := N * N * N

	idx := b*cube*cube + g*cube + r
	x := idx % size
	y := idx / size

	min := h.Image.Bounds().Min
	return h.Image.At(min.X+x, min.Y+y)
}

// colorToFloat64 converts an image color to float64 RGB values in range [0, 1]
func colorToFloat64(c color.Color) (r, g, b float64) {
	rVal, gVal, bVal, _ := c.RGBA()
	// Convert from uint32 (0-65535) to float64 (0-1)
	return float64(rVal) / 65535.0, float64(gVal) / 65535.0, float64(bVal) / 65535.0
}

// interpolateSamples linearly interpolates between two colors
func interpolateSamples(c1, c2 color.Color, t float64) (r, g, b float64) {
	r1, g1, b1 := colorToFloat64(c1)
	r2, g2, b2 := colorToFloat64(c2)

	return r1 + t*(r2-r1), g1 + t*(g2-g1), b1 + t*(b2-b1)
}

// Interpolate performs trilinear interpolation in the 3D HALD LUT
func (h HALD) Interpolate(r, g, b float64) (float64, float64, float64) {
	cubeF := float64(h.level*h.level - 1) // N² - 1

	rIdx := r * cubeF
	gIdx := g * cubeF
	bIdx := b * cubeF

	r0 := int(math.Floor(rIdx))
	r1 := min(r0+1, h.level*h.level-1)
	g0 := int(math.Floor(gIdx))
	g1 := min(g0+1, h.level*h.level-1)
	b0 := int(math.Floor(bIdx))
	b1 := min(b0+1, h.level*h.level-1)

	rFrac := rIdx - float64(r0)
	gFrac := gIdx - float64(g0)
	bFrac := bIdx - float64(b0)

	c000 := h.sample(r0, g0, b0)
	c001 := h.sample(r0, g0, b1)
	c010 := h.sample(r0, g1, b0)
	c011 := h.sample(r0, g1, b1)
	c100 := h.sample(r1, g0, b0)
	c101 := h.sample(r1, g0, b1)
	c110 := h.sample(r1, g1, b0)
	c111 := h.sample(r1, g1, b1)

	c00r, c00g, c00b := interpolateSamples(c000, c100, rFrac)
	c01r, c01g, c01b := interpolateSamples(c001, c101, rFrac)
	c10r, c10g, c10b := interpolateSamples(c010, c110, rFrac)
	c11r, c11g, c11b := interpolateSamples(c011, c111, rFrac)

	c0r, c0g, c0b := lerp(c00r, c00g, c00b, c10r, c10g, c10b, gFrac)
	c1r, c1g, c1b := lerp(c01r, c01g, c01b, c11r, c11g, c11b, gFrac)

	return lerp(c0r, c0g, c0b, c1r, c1g, c1b, bFrac)
}

// lerp linearly interpolates between two RGB values
func lerp(r1, g1, b1, r2, g2, b2, t float64) (float64, float64, float64) {
	return r1 + t*(r2-r1), g1 + t*(g2-g1), b1 + t*(b2-b1)
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

// Apply applies the HALD LUT to an image with full intensity (1.0)
func (h HALD) Apply(img image.Image) *image.RGBA {
	return h.ApplyScaled(img, 1.0)
}

// ApplyScaled applies the HALD LUT to an image with adjustable intensity
func (h HALD) ApplyScaled(img image.Image, intensity float64) *image.RGBA {
	bounds := img.Bounds()
	out := image.NewRGBA(bounds)

	// Clamp intensity to [0, 1]
	intensity = max(0, min(1, intensity))

	var wg sync.WaitGroup

	// Process each row in parallel
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		wg.Add(1)
		go func(y int) {
			defer wg.Done()
			h.processRowScaled(img, out, bounds, y, intensity)
		}(y)
	}

	wg.Wait()
	return out
}

// processRowScaled processes a single row of the image with intensity blending
func (h HALD) processRowScaled(img image.Image, out *image.RGBA, bounds image.Rectangle, y int, intensity float64) {
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		r, g, b, a := img.At(x, y).RGBA()

		// Convert from uint32 (0-65535) to float64 (0-1)
		rNorm := float64(r) / 65535.0
		gNorm := float64(g) / 65535.0
		bNorm := float64(b) / 65535.0

		// Apply HALD using trilinear interpolation
		resultR, resultG, resultB := h.Interpolate(rNorm, gNorm, bNorm)

		// Blend between original (identity) and HALD result
		blendedR := rNorm*(1-intensity) + resultR*intensity
		blendedG := gNorm*(1-intensity) + resultG*intensity
		blendedB := bNorm*(1-intensity) + resultB*intensity

		// Clamp to [0, 1]
		blendedR = max(0, min(1, blendedR))
		blendedG = max(0, min(1, blendedG))
		blendedB = max(0, min(1, blendedB))

		// Convert back to uint8
		out.SetRGBA(x, y, color.RGBA{
			R: uint8(blendedR * 255),
			G: uint8(blendedG * 255),
			B: uint8(blendedB * 255),
			A: uint8(a / 257), // Convert from uint32 to uint8
		})
	}
}

// Blend does a weighted blend of two HALDs using the two intensities
// i1 and i2 provided in input.
func (h *HALD) Blend(h2 HALD, i1, i2 float64) (*HALD, error) {
	// Validate levels match
	if h.level != h2.level {
		return h, ErrDifferentLevels
	}

	bounds := h.Image.Bounds()
	blended := image.NewRGBA(bounds)

	total := i1 + i2
	w1 := i1 / total
	w2 := i2 / total

	// Blend each pixel
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c1 := h.Image.At(x, y)
			c2 := h2.Image.At(x, y)

			r1, g1, b1 := colorToFloat64(c1)
			r2, g2, b2 := colorToFloat64(c2)

			// Blend the colors
			r := r1*w1 + r2*w2
			g := g1*w1 + g2*w2
			b := b1*w1 + b2*w2

			blended.SetRGBA(x, y, color.RGBA{
				R: uint8(r * 255),
				G: uint8(g * 255),
				B: uint8(b * 255),
				A: 255,
			})
		}
	}

	result, err := newHALD(blended)
	if err != nil {
		return h, err
	}

	return &result, nil
}

// WriteTo writes the HALD image as PNG to the given writer
func (h HALD) WriteTo(w io.Writer) (int64, error) {
	return 0, png.Encode(w, h.Image)
}

// Identity creates a neutral/identity HALD of the given level.
// An identity HALD returns each input color unchanged.
func Identity(level int) HALD {
	N := level
	cube := N * N     // samples per axis
	size := N * N * N // image width & height
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	den := float64(cube - 1)

	for b := 0; b < cube; b++ {
		for g := 0; g < cube; g++ {
			for r := 0; r < cube; r++ {
				idx := b*cube*cube + g*cube + r
				x := idx % size
				y := idx / size

				R := uint8((float64(r) / den) * 255.0)
				G := uint8((float64(g) / den) * 255.0)
				B := uint8((float64(b) / den) * 255.0)
				img.SetRGBA(x, y, color.RGBA{R: R, G: G, B: B, A: 255})
			}
		}
	}
	return HALD{Image: img, level: N}
}

// Load reads a HALD LUT from a PNG image reader
func Load(r io.Reader) (HALD, error) {
	img, err := png.Decode(r)
	if err != nil {
		return HALD{}, err
	}

	return newHALD(img)
}

// LoadFile reads a HALD LUT from a PNG file
func LoadFile(path string) (HALD, error) {
	f, err := os.Open(path)
	if err != nil {
		return HALD{}, err
	}
	defer f.Close()

	return Load(f)
}

// Level returns the HALD level of this LUT
func (h HALD) Level() int {
	return h.level
}
