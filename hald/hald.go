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
	img   image.Image
	level int
}

var (
	ErrInvalidDimensions = errors.New("invalid HALD image dimensions")
	ErrNilImage          = errors.New("image is nil")
)

// newHALD creates a HALD from an image after validating dimensions
func newHALD(img image.Image) (HALD, error) {
	if img == nil {
		return HALD{}, ErrNilImage
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// HALD level N has dimensions (N³) × (N²)
	// So: width = N³ and height = N²
	// Therefore: level = height^(1/2) and width should equal level^3

	level := int(math.Round(math.Sqrt(float64(height))))

	// Validate dimensions
	if level*level != height || level*level*level != width {
		return HALD{}, ErrInvalidDimensions
	}

	return HALD{
		img:   img,
		level: level,
	}, nil
}

// getSample retrieves the color at the given 3D cube coordinates
// r, g, b should be in range [0, level-1]
func (h HALD) getSample(r, g, b int) color.Color {
	// Calculate 2D position in the HALD image
	// The HALD layout uses a specific arrangement
	// For level N: the image is N³ × N² pixels
	// Each color is indexed by: x = b*level + r, y = g*level + index_in_g_section

	x := b*h.level + r
	y := g * h.level

	return h.img.At(x+h.img.Bounds().Min.X, y+h.img.Bounds().Min.Y)
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

// interpolate performs trilinear interpolation in the 3D HALD LUT
func (h HALD) interpolate(r, g, b float64) (float64, float64, float64) {
	levelF := float64(h.level - 1)

	// Normalize input to HALD coordinates [0, level-1]
	rIdx := r * levelF
	gIdx := g * levelF
	bIdx := b * levelF

	// Find the surrounding cube vertices
	r0 := int(rIdx)
	g0 := int(gIdx)
	b0 := int(bIdx)

	r1 := min(r0+1, h.level-1)
	g1 := min(g0+1, h.level-1)
	b1 := min(b0+1, h.level-1)

	// Calculate interpolation weights
	rFrac := rIdx - float64(r0)
	gFrac := gIdx - float64(g0)
	bFrac := bIdx - float64(b0)

	// Get the 8 corner samples
	c000 := h.getSample(r0, g0, b0)
	c001 := h.getSample(r0, g0, b1)
	c010 := h.getSample(r0, g1, b0)
	c011 := h.getSample(r0, g1, b1)
	c100 := h.getSample(r1, g0, b0)
	c101 := h.getSample(r1, g0, b1)
	c110 := h.getSample(r1, g1, b0)
	c111 := h.getSample(r1, g1, b1)

	// Trilinear interpolation
	// First interpolate along r
	c00r, c00g, c00b := interpolateSamples(c000, c100, rFrac)
	c01r, c01g, c01b := interpolateSamples(c001, c101, rFrac)
	c10r, c10g, c10b := interpolateSamples(c010, c110, rFrac)
	c11r, c11g, c11b := interpolateSamples(c011, c111, rFrac)

	// Then interpolate along g
	c0r, c0g, c0b := lerp(c00r, c00g, c00b, c10r, c10g, c10b, gFrac)
	c1r, c1g, c1b := lerp(c01r, c01g, c01b, c11r, c11g, c11b, gFrac)

	// Finally interpolate along b
	return lerp(c0r, c0g, c0b, c1r, c1g, c1b, bFrac)
}

// lerp linearly interpolates between two RGB values
func lerp(r1, g1, b1, r2, g2, b2, t float64) (float64, float64, float64) {
	return r1 + t*(r2-r1), g1 + t*(g2-g1), b1 + t*(b2-b1)
}

func min[C int | float64](a, b C) C {
	if a < b {
		return a
	}
	return b
}

func max[C int | float64](a, b C) C {
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
		resultR, resultG, resultB := h.interpolate(rNorm, gNorm, bNorm)

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

// Identity creates a neutral/identity HALD of the given level.
// An identity HALD returns each input color unchanged.
func Identity(level int) HALD {
	width := level * level * level
	height := level * level
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	levelF := float64(level - 1)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := x % level
			g := y / level
			b := x / level

			// Normalize to 0-1 range, then to 0-255
			rVal := uint8((float64(r) / levelF) * 255)
			gVal := uint8((float64(g) / levelF) * 255)
			bVal := uint8((float64(b) / levelF) * 255)

			img.SetRGBA(x, y, color.RGBA{R: rVal, G: gVal, B: bVal, A: 255})
		}
	}

	return HALD{img: img, level: level}
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

// Bounds returns the image bounds of the HALD LUT
func (h HALD) Bounds() image.Rectangle {
	return h.img.Bounds()
}
