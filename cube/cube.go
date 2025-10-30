package cube

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"strings"
	"sync"
)

type Sample struct {
	R, G, B float64
}

func (s *Sample) Sum(s2 Sample) *Sample {
	s.R += s2.R
	s.G += s2.G
	s.B += s2.B
	return s
}

func (s *Sample) Blend(s2 Sample, w1, w2 float64) *Sample {
	s.R = s.R*w1 + s2.R*w2
	s.G = s.G*w1 + s2.G*w2
	s.B = s.B*w1 + s2.B*w2
	return s
}

func (s *Sample) Clamp(min, max Sample) *Sample {
	s.R = (s.R - min.R) / (max.R - min.R)
	s.G = (s.G - min.G) / (max.G - min.G)
	s.B = (s.B - min.B) / (max.B - min.B)
	return s
}

func (s *Sample) Scale(v float64) *Sample {
	s.R *= v
	s.G *= v
	s.B *= v
	return s
}

func (s Sample) String() string {
	return fmt.Sprintf("%f %f %f", s.R, s.G, s.B)
}

func (s *Sample) Rescale(minVal, maxVal float64, domainMin, domainMax Sample) *Sample {
	if maxVal == minVal {
		s.R = domainMin.R + (domainMax.R-domainMin.R)/2
		s.G = domainMin.G + (domainMax.G-domainMin.G)/2
		s.B = domainMin.B + (domainMax.B-domainMin.B)/2
		return s
	}

	globalRange := maxVal - minVal
	s.R = domainMin.R + (s.R-minVal)/globalRange*(domainMax.R-domainMin.R)
	s.G = domainMin.G + (s.G-minVal)/globalRange*(domainMax.G-domainMin.G)
	s.B = domainMin.B + (s.B-minVal)/globalRange*(domainMax.B-domainMin.B)
	return s
}

type Cube struct {
	Title     string
	Meta      string
	LUT3Dsize int
	DomainMin Sample
	DomainMax Sample
	Samples   []Sample
}

func (c Cube) String() string {
	var buf strings.Builder

	c.WriteTo(&buf)
	return buf.String()
}

func (c Cube) WriteTo(w io.Writer) (n int64, err error) {
	var cur int

	if c.Title != "" {
		if cur, err = fmt.Fprintf(w, "TITLE \"%s\"\n", c.Title); err != nil {
			return
		}
		n += int64(cur)
	}
	if c.Meta != "" {
		if cur, err = fmt.Fprintf(w, "%s\n", c.Meta); err != nil {
			return
		}
		n += int64(cur)
	}

	if cur, err = fmt.Fprintf(w, "LUT_3D_SIZE %d\n\n", c.LUT3Dsize); err != nil {
		return
	}
	n += int64(cur)

	if cur, err = fmt.Fprintf(w, "DOMAIN_MIN %v\n", c.DomainMin); err != nil {
		return
	}
	n += int64(cur)
	if cur, err = fmt.Fprintf(w, "DOMAIN_MAX %v\n\n", c.DomainMax); err != nil {
		return
	}
	n += int64(cur)

	for _, s := range c.Samples {
		if cur, err = fmt.Fprintln(w, s); err != nil {
			return
		}
		n += int64(cur)
	}

	return
}

func (c *Cube) Scale(v float64) *Cube {
	if v <= 0 || v > 1 {
		return c
	}

	for i := range c.Samples {
		c.Samples[i].Scale(v)
	}
	return c
}

func (c *Cube) Clamp() *Cube {
	for i := range c.Samples {
		c.Samples[i].Clamp(c.DomainMin, c.DomainMax)
	}
	return c
}

var (
	ErrEmptyLut            = errors.New("empty LUT")
	ErrDifferentSampleSize = errors.New("different sample sizes in LUTs")
	ErrUnrecognisedLine    = errors.New("unrecognised line")
)

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (c *Cube) Sum(c2 Cube) (*Cube, error) {
	if len(c.Samples) == 0 || len(c2.Samples) == 0 {
		return c, ErrEmptyLut
	}

	if len(c.Samples) != len(c2.Samples) {
		return c, ErrDifferentSampleSize
	}

	for i := range c.Samples {
		c.Samples[i].Sum(c2.Samples[i])
	}
	return c, nil
}

// Blend does a weighted blend of two LUTs using the two intensities
// i1 and i2 provided in input.
func (c *Cube) Blend(c2 Cube, i1, i2 float64) (*Cube, error) {
	if len(c.Samples) == 0 || len(c2.Samples) == 0 {
		return c, ErrEmptyLut
	}

	if len(c.Samples) != len(c2.Samples) {
		return c, ErrDifferentSampleSize
	}

	total := i1 + i2
	w1 := i1 / total
	w2 := i2 / total

	for i := range c.Samples {
		c.Samples[i].Blend(c2.Samples[i], w1, w2)
	}
	return c, nil
}

func (c *Cube) MustBlend(c2 Cube, i1, i2 float64) *Cube {
	ret, err := c.Blend(c2, i1, i2)
	if err != nil {
		panic(err)
	}
	return ret
}

func (c *Cube) MustSum(c2 Cube) *Cube {
	ret, err := c.Sum(c2)
	if err != nil {
		panic(err)
	}
	return ret
}

func (c Cube) minmax() (minVal, maxVal float64) {
	if len(c.Samples) == 0 {
		return 0, 1
	}

	minVal = c.Samples[0].R
	maxVal = c.Samples[0].R
	for _, s := range c.Samples {
		minVal = min(minVal, min(min(s.R, s.G), s.B))
		maxVal = max(maxVal, max(max(s.R, s.G), s.B))
	}
	return
}

func (c *Cube) Rescale() *Cube {
	minVal, maxVal := c.minmax()

	for i := range c.Samples {
		c.Samples[i].Rescale(minVal, maxVal, c.DomainMin, c.DomainMax)
	}

	return c
}

// interpolate performs trilinear interpolation in the 3D LUT
func (c Cube) interpolate(r, g, b float64) Sample {
	size := float64(c.LUT3Dsize - 1)

	// Normalize input to cube coordinates [0, size]
	rIdx := (r - c.DomainMin.R) / (c.DomainMax.R - c.DomainMin.R) * size
	gIdx := (g - c.DomainMin.G) / (c.DomainMax.G - c.DomainMin.G) * size
	bIdx := (b - c.DomainMin.B) / (c.DomainMax.B - c.DomainMin.B) * size

	// Clamp to valid range
	rIdx = max(0, min(size, rIdx))
	gIdx = max(0, min(size, gIdx))
	bIdx = max(0, min(size, bIdx))

	// Find the surrounding cube vertices
	r0 := int(rIdx)
	g0 := int(gIdx)
	b0 := int(bIdx)

	r1 := min(float64(r0+1), size)
	g1 := min(float64(g0+1), size)
	b1 := min(float64(b0+1), size)

	// Calculate interpolation weights
	rFrac := rIdx - float64(r0)
	gFrac := gIdx - float64(g0)
	bFrac := bIdx - float64(b0)

	// Get the 8 corner samples
	c000 := c.getSample(r0, g0, b0)
	c001 := c.getSample(r0, g0, int(b1))
	c010 := c.getSample(r0, int(g1), b0)
	c011 := c.getSample(r0, int(g1), int(b1))
	c100 := c.getSample(int(r1), g0, b0)
	c101 := c.getSample(int(r1), g0, int(b1))
	c110 := c.getSample(int(r1), int(g1), b0)
	c111 := c.getSample(int(r1), int(g1), int(b1))

	// Trilinear interpolation
	// First interpolate along r
	c00 := interpolateSamples(c000, c100, rFrac)
	c01 := interpolateSamples(c001, c101, rFrac)
	c10 := interpolateSamples(c010, c110, rFrac)
	c11 := interpolateSamples(c011, c111, rFrac)

	// Then interpolate along g
	c0 := interpolateSamples(c00, c10, gFrac)
	c1 := interpolateSamples(c01, c11, gFrac)

	// Finally interpolate along b
	return interpolateSamples(c0, c1, bFrac)
}

// getSample retrieves a sample from the 3D LUT at the given indices
func (c Cube) getSample(r, g, b int) Sample {
	idx := r + g*c.LUT3Dsize + b*c.LUT3Dsize*c.LUT3Dsize
	if idx >= len(c.Samples) {
		return Sample{R: 0, G: 0, B: 0}
	}
	return c.Samples[idx]
}

// interpolateSamples linearly interpolates between two samples
func interpolateSamples(s1, s2 Sample, t float64) Sample {
	return Sample{
		R: s1.R + t*(s2.R-s1.R),
		G: s1.G + t*(s2.G-s1.G),
		B: s1.B + t*(s2.B-s1.B),
	}
}

func (c Cube) Apply(img image.Image) *image.RGBA {
	return c.ApplyScaled(img, 1.0)
}

func (c Cube) ApplyScaled(img image.Image, intensity float64) *image.RGBA {
	bounds := img.Bounds()
	out := image.NewRGBA(bounds)

	// Clamp intensity to [0, 1]
	intensity = max(0, min(1, intensity))

	// Pre-compute domain ranges to avoid recalculation
	domainRangeR := c.DomainMax.R - c.DomainMin.R
	domainRangeG := c.DomainMax.G - c.DomainMin.G
	domainRangeB := c.DomainMax.B - c.DomainMin.B

	var wg sync.WaitGroup

	// Process each row in parallel
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		wg.Go(func() {
			c.processRowScaled(
				img,
				out,
				bounds,
				y,
				domainRangeR,
				domainRangeG,
				domainRangeB,
				intensity,
			)
		})
	}

	wg.Wait()
	return out
}

// processRowScaled processes a single row of the image with intensity blending
func (c Cube) processRowScaled(img image.Image, out *image.RGBA, bounds image.Rectangle, y int, domainRangeR, domainRangeG, domainRangeB, intensity float64) {
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		r, g, b, a := img.At(x, y).RGBA()

		// Convert from uint32 (0-65535) to float64 (0-1)
		rNorm := float64(r) / 65535.0
		gNorm := float64(g) / 65535.0
		bNorm := float64(b) / 65535.0

		// Map to LUT domain
		rLut := c.DomainMin.R + rNorm*domainRangeR
		gLut := c.DomainMin.G + gNorm*domainRangeG
		bLut := c.DomainMin.B + bNorm*domainRangeB

		// Apply LUT using trilinear interpolation
		result := c.interpolate(rLut, gLut, bLut)

		// Blend between original (identity) and LUT result
		// Identity in LUT domain space is just the input color
		blendedR := rLut*(1-intensity) + result.R*intensity
		blendedG := gLut*(1-intensity) + result.G*intensity
		blendedB := bLut*(1-intensity) + result.B*intensity

		// Map back from LUT domain to [0, 1]
		rOut := (blendedR - c.DomainMin.R) / domainRangeR
		gOut := (blendedG - c.DomainMin.G) / domainRangeG
		bOut := (blendedB - c.DomainMin.B) / domainRangeB

		// Clamp to [0, 1]
		rOut = max(0, min(1, rOut))
		gOut = max(0, min(1, gOut))
		bOut = max(0, min(1, bOut))

		// Convert back to uint8
		out.SetRGBA(x, y, color.RGBA{
			R: uint8(rOut * 255),
			G: uint8(gOut * 255),
			B: uint8(bOut * 255),
			A: uint8(a / 257), // Convert from uint32 to uint8
		})
	}
}

func Load(r io.Reader) (Cube, error) {
	var (
		c       Cube
		scanner = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Get the first field to determine line type
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch field := fields[0]; {
		case field == "TITLE":
			// Extract quoted title
			if start := strings.Index(line, "\""); start != -1 {
				if end := strings.LastIndex(line, "\""); end > start {
					c.Title = line[start+1 : end]
				}
			}

		case field == "LUT_3D_SIZE":
			if _, err := fmt.Sscanf(line, "LUT_3D_SIZE %d", &c.LUT3Dsize); err != nil {
				return Cube{}, err
			}

		case field == "DOMAIN_MIN":
			if _, err := fmt.Sscanf(
				line,
				"DOMAIN_MIN %f %f %f",
				&c.DomainMin.R,
				&c.DomainMin.G,
				&c.DomainMin.B,
			); err != nil {
				return Cube{}, err
			}

		case field == "DOMAIN_MAX":
			if _, err := fmt.Sscanf(
				line,
				"DOMAIN_MAX %f %f %f",
				&c.DomainMax.R,
				&c.DomainMax.G,
				&c.DomainMax.B,
			); err != nil {
				return Cube{}, err
			}

		// Metadata lines (starting with #)
		case strings.HasPrefix(line, "#"):
			if c.Meta != "" {
				c.Meta += "\n"
			}
			c.Meta += line

		case len(fields) == 3:
			var s Sample
			if _, err := fmt.Sscanf(line, "%f %f %f", &s.R, &s.G, &s.B); err != nil {
				return Cube{}, err
			}
			c.Samples = append(c.Samples, s)

		default:
			return Cube{}, ErrUnrecognisedLine
		}
	}

	if err := scanner.Err(); err != nil {
		return c, err
	}

	return c, nil
}

func LoadFile(path string) (Cube, error) {
	f, err := os.Open(path)
	if err != nil {
		return Cube{}, err
	}
	defer f.Close()

	return Load(f)
}
