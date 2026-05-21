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

func (s Sample) String() string {
	return fmt.Sprintf("%f %f %f", s.R, s.G, s.B)
}

func sumSamples(s1, s2 Sample) Sample {
	return Sample{
		R: s1.R + s2.R,
		G: s1.G + s2.G,
		B: s1.B + s2.B,
	}
}

func blendSamples(s1, s2 Sample, w1, w2 float64) Sample {
	return Sample{
		R: s1.R*w1 + s2.R*w2,
		G: s1.G*w1 + s2.G*w2,
		B: s1.B*w1 + s2.B*w2,
	}
}

func clampSample(s, dmin, dmax Sample) Sample {
	return Sample{
		R: (s.R - dmin.R) / (dmax.R - dmin.R),
		G: (s.G - dmin.G) / (dmax.G - dmin.G),
		B: (s.B - dmin.B) / (dmax.B - dmin.B),
	}
}

func scaleSample(s Sample, v float64) Sample {
	return Sample{R: s.R * v, G: s.G * v, B: s.B * v}
}

func rescaleSample(s Sample, minVal, maxVal float64, dmin, dmax Sample) Sample {
	if maxVal == minVal {
		return Sample{
			R: dmin.R + (dmax.R-dmin.R)/2,
			G: dmin.G + (dmax.G-dmin.G)/2,
			B: dmin.B + (dmax.B-dmin.B)/2,
		}
	}
	r := maxVal - minVal
	return Sample{
		R: dmin.R + (s.R-minVal)/r*(dmax.R-dmin.R),
		G: dmin.G + (s.G-minVal)/r*(dmax.G-dmin.G),
		B: dmin.B + (s.B-minVal)/r*(dmax.B-dmin.B),
	}
}

func interpolateSamples(s1, s2 Sample, t float64) Sample {
	return Sample{
		R: s1.R + t*(s2.R-s1.R),
		G: s1.G + t*(s2.G-s1.G),
		B: s1.B + t*(s2.B-s1.B),
	}
}

type Cube struct {
	Title     string
	Meta      string
	LUT3Dsize int
	DomainMin Sample
	DomainMax Sample
	Samples   []Sample
}

var (
	ErrEmptyLut            = errors.New("empty LUT")
	ErrDifferentSampleSize = errors.New("different sample sizes in LUTs")
	ErrInvalidDomain       = errors.New("invalid domain (degenerate range)")
	ErrInvalidScale        = errors.New("scale must be in (0, 1]")
	ErrUnrecognisedLine    = errors.New("unrecognised line")
)

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

func (c *Cube) Scale(v float64) error {
	if v <= 0 || v > 1 {
		return ErrInvalidScale
	}
	for i := range c.Samples {
		c.Samples[i] = scaleSample(c.Samples[i], v)
	}
	return nil
}

func (c *Cube) Clamp() error {
	if len(c.Samples) == 0 {
		return ErrEmptyLut
	}
	if c.DomainMax.R == c.DomainMin.R ||
		c.DomainMax.G == c.DomainMin.G ||
		c.DomainMax.B == c.DomainMin.B {
		return ErrInvalidDomain
	}
	for i := range c.Samples {
		c.Samples[i] = clampSample(c.Samples[i], c.DomainMin, c.DomainMax)
	}
	return nil
}

func (c *Cube) Sum(c2 Cube) error {
	if len(c.Samples) == 0 || len(c2.Samples) == 0 {
		return ErrEmptyLut
	}
	if len(c.Samples) != len(c2.Samples) {
		return ErrDifferentSampleSize
	}
	for i := range c.Samples {
		c.Samples[i] = sumSamples(c.Samples[i], c2.Samples[i])
	}
	return nil
}

// Blend replaces c with the weighted blend of c and c2 using intensities i1, i2.
func (c *Cube) Blend(c2 Cube, i1, i2 float64) error {
	if len(c.Samples) == 0 || len(c2.Samples) == 0 {
		return ErrEmptyLut
	}
	if len(c.Samples) != len(c2.Samples) {
		return ErrDifferentSampleSize
	}

	total := i1 + i2
	if total == 0 {
		return nil
	}
	w1 := i1 / total
	w2 := i2 / total

	for i := range c.Samples {
		c.Samples[i] = blendSamples(c.Samples[i], c2.Samples[i], w1, w2)
	}
	return nil
}

func (c Cube) minmax() (mn, mx float64) {
	if len(c.Samples) == 0 {
		return 0, 1
	}
	mn = c.Samples[0].R
	mx = c.Samples[0].R
	for _, s := range c.Samples {
		mn = min(mn, min(min(s.R, s.G), s.B))
		mx = max(mx, max(max(s.R, s.G), s.B))
	}
	return
}

func (c *Cube) Rescale() {
	mn, mx := c.minmax()
	for i := range c.Samples {
		c.Samples[i] = rescaleSample(c.Samples[i], mn, mx, c.DomainMin, c.DomainMax)
	}
}

func (c Cube) sample(r, g, b int) Sample {
	idx := r + g*c.LUT3Dsize + b*c.LUT3Dsize*c.LUT3Dsize
	if idx >= len(c.Samples) {
		return Sample{}
	}
	return c.Samples[idx]
}

func (c Cube) interpolate(r, g, b float64) Sample {
	size := c.LUT3Dsize - 1
	sizeF := float64(size)

	rIdx := (r - c.DomainMin.R) / (c.DomainMax.R - c.DomainMin.R) * sizeF
	gIdx := (g - c.DomainMin.G) / (c.DomainMax.G - c.DomainMin.G) * sizeF
	bIdx := (b - c.DomainMin.B) / (c.DomainMax.B - c.DomainMin.B) * sizeF

	rIdx = max(0, min(sizeF, rIdx))
	gIdx = max(0, min(sizeF, gIdx))
	bIdx = max(0, min(sizeF, bIdx))

	r0, g0, b0 := int(rIdx), int(gIdx), int(bIdx)
	r1, g1, b1 := min(r0+1, size), min(g0+1, size), min(b0+1, size)

	rFrac := rIdx - float64(r0)
	gFrac := gIdx - float64(g0)
	bFrac := bIdx - float64(b0)

	return interpolateSamples(
		interpolateSamples(
			interpolateSamples(c.sample(r0, g0, b0), c.sample(r1, g0, b0), rFrac),
			interpolateSamples(c.sample(r0, g1, b0), c.sample(r1, g1, b0), rFrac),
			gFrac,
		),
		interpolateSamples(
			interpolateSamples(c.sample(r0, g0, b1), c.sample(r1, g0, b1), rFrac),
			interpolateSamples(c.sample(r0, g1, b1), c.sample(r1, g1, b1), rFrac),
			gFrac,
		),
		bFrac,
	)
}

func (c Cube) Apply(img image.Image) *image.RGBA {
	return c.ApplyScaled(img, 1.0)
}

func (c Cube) ApplyScaled(img image.Image, intensity float64) *image.RGBA {
	bounds := img.Bounds()
	out := image.NewRGBA(bounds)

	intensity = max(0, min(1, intensity))

	dr := c.DomainMax.R - c.DomainMin.R
	dg := c.DomainMax.G - c.DomainMin.G
	db := c.DomainMax.B - c.DomainMin.B

	var wg sync.WaitGroup
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		wg.Go(func() {
			c.applyRow(img, out, bounds, y, dr, dg, db, intensity)
		})
	}
	wg.Wait()
	return out
}

func (c Cube) applyRow(img image.Image, out *image.RGBA, bounds image.Rectangle, y int, dr, dg, db, intensity float64) {
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		r32, g32, b32, a32 := img.At(x, y).RGBA()

		r := float64(r32) / 65535.0
		g := float64(g32) / 65535.0
		b := float64(b32) / 65535.0

		s := c.interpolate(
			r*dr+c.DomainMin.R,
			g*dg+c.DomainMin.G,
			b*db+c.DomainMin.B,
		)

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

func Load(r io.Reader) (Cube, error) {
	var (
		c       = Cube{DomainMax: Sample{1, 1, 1}}
		scanner = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch field := fields[0]; {
		case field == "TITLE":
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
