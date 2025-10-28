package cube

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
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
