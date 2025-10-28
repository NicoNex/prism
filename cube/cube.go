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

func (s Sample) Blend(s2 Sample) Sample {
	return Sample{
		R: s.R + s2.R,
		G: s.G + s2.G,
		B: s.B + s2.B,
	}
}

func (s Sample) Clamp(min, max Sample) Sample {
	return Sample{
		R: (s.R - min.R) / (max.R - min.R),
		G: (s.G - min.G) / (max.G - min.G),
		B: (s.B - min.B) / (max.B - min.B),
	}
}

func (s Sample) Scale(v float64) Sample {
	return Sample{
		R: s.R * v,
		G: s.G * v,
		B: s.B * v,
	}
}

func (s Sample) String() string {
	return fmt.Sprintf("%f %f %f", s.R, s.G, s.B)
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

func (c Cube) Scale(v float64) Cube {
	if v <= 0 || v > 1 {
		return c
	}

	for i, s := range c.Samples {
		c.Samples[i] = s.Scale(v)
	}
	return c
}

func (c Cube) Clamp() Cube {
	for i, s := range c.Samples {
		c.Samples[i] = s.Clamp(c.DomainMin, c.DomainMax)
	}
	return c
}

var (
	ErrEmptyLut            = errors.New("empty LUT")
	ErrDifferentSampleSize = errors.New("different sample sizes in LUTs")
	ErrUnrecognisedLine    = errors.New("unrecognised line")
)

func (c Cube) Blend(c2 Cube) (Cube, error) {
	if len(c.Samples) == 0 || len(c2.Samples) == 0 {
		return c, ErrEmptyLut
	}

	if len(c.Samples) != len(c2.Samples) {
		return c, ErrDifferentSampleSize
	}

	for i, s := range c.Samples {
		c.Samples[i] = s.Blend(c2.Samples[i])
	}
	return c, nil
}

func (c Cube) MustBlend(c2 Cube) Cube {
	ret, err := c.Blend(c2)
	if err != nil {
		panic(err)
	}
	return ret
}

func parseSample(line, prefix string, s *Sample) error {
	var r, g, b float64
	_, err := fmt.Sscanf(line, prefix+" %f %f %f", &r, &g, &b)
	if err == nil {
		*s = Sample{R: r, G: g, B: b}
	}
	return err
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
	return Load(f)
}
