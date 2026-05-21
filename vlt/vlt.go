package vlt

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

const maxVal = 4095

// Sample is a 12-bit RGB triple in range [0, 4095].
type Sample struct {
	R, G, B uint16
}

// VLT is a Panasonic Varicam 3D LUT with 12-bit integer samples.
type VLT struct {
	LUT3Dsize int
	Samples   []Sample
}

var (
	ErrEmptyLut            = errors.New("empty LUT")
	ErrDifferentSampleSize = errors.New("different sample sizes in LUTs")
	ErrInvalidScale        = errors.New("scale must be in (0, 1]")
	ErrUnrecognisedLine    = errors.New("unrecognised line")
)

func sumSamples(s1, s2 Sample) Sample {
	return Sample{
		R: uint16(min(int(s1.R)+int(s2.R), maxVal)),
		G: uint16(min(int(s1.G)+int(s2.G), maxVal)),
		B: uint16(min(int(s1.B)+int(s2.B), maxVal)),
	}
}

func blendSamples(s1, s2 Sample, w1, w2 float64) Sample {
	return Sample{
		R: uint16(float64(s1.R)*w1 + float64(s2.R)*w2 + 0.5),
		G: uint16(float64(s1.G)*w1 + float64(s2.G)*w2 + 0.5),
		B: uint16(float64(s1.B)*w1 + float64(s2.B)*w2 + 0.5),
	}
}

func scaleSample(s Sample, v float64) Sample {
	return Sample{
		R: uint16(float64(s.R)*v + 0.5),
		G: uint16(float64(s.G)*v + 0.5),
		B: uint16(float64(s.B)*v + 0.5),
	}
}

func rescaleSample(s Sample, mn, mx int) Sample {
	if mx == mn {
		return Sample{R: maxVal / 2, G: maxVal / 2, B: maxVal / 2}
	}
	r := float64(mx - mn)
	return Sample{
		R: uint16((float64(s.R)-float64(mn))/r*maxVal + 0.5),
		G: uint16((float64(s.G)-float64(mn))/r*maxVal + 0.5),
		B: uint16((float64(s.B)-float64(mn))/r*maxVal + 0.5),
	}
}

// rgb is a float64 triple used for interpolation to avoid rounding in nested lerps.
type rgb struct{ r, g, b float64 }

func toRGB(s Sample) rgb {
	return rgb{float64(s.R), float64(s.G), float64(s.B)}
}

func lerpRGB(a, b rgb, t float64) rgb {
	return rgb{
		a.r + t*(b.r-a.r),
		a.g + t*(b.g-a.g),
		a.b + t*(b.b-a.b),
	}
}

func (v VLT) WriteTo(w io.Writer) (n int64, err error) {
	var cur int

	for _, l := range []string{
		"# panasonic vlt file version 1.0\n",
		"# source vlt file \"\"\n",
		fmt.Sprintf("LUT_3D_SIZE %d\n", v.LUT3Dsize),
		"\n",
	} {
		if cur, err = fmt.Fprint(w, l); err != nil {
			return
		}
		n += int64(cur)
	}

	for _, s := range v.Samples {
		if cur, err = fmt.Fprintf(w, "%d %d %d\n", s.R, s.G, s.B); err != nil {
			return
		}
		n += int64(cur)
	}

	return
}

func (v *VLT) Scale(val float64) error {
	if val <= 0 || val > 1 {
		return ErrInvalidScale
	}
	for i := range v.Samples {
		v.Samples[i] = scaleSample(v.Samples[i], val)
	}
	return nil
}

func (v *VLT) Sum(v2 VLT) error {
	if len(v.Samples) == 0 || len(v2.Samples) == 0 {
		return ErrEmptyLut
	}
	if len(v.Samples) != len(v2.Samples) {
		return ErrDifferentSampleSize
	}
	for i := range v.Samples {
		v.Samples[i] = sumSamples(v.Samples[i], v2.Samples[i])
	}
	return nil
}

// Blend replaces v with the weighted blend of v and v2 using intensities i1, i2.
func (v *VLT) Blend(v2 VLT, i1, i2 float64) error {
	if len(v.Samples) == 0 || len(v2.Samples) == 0 {
		return ErrEmptyLut
	}
	if len(v.Samples) != len(v2.Samples) {
		return ErrDifferentSampleSize
	}

	total := i1 + i2
	if total == 0 {
		return nil
	}
	w1 := i1 / total
	w2 := i2 / total

	for i := range v.Samples {
		v.Samples[i] = blendSamples(v.Samples[i], v2.Samples[i], w1, w2)
	}
	return nil
}

func (v VLT) minmax() (mn, mx int) {
	if len(v.Samples) == 0 {
		return 0, maxVal
	}
	mn = int(v.Samples[0].R)
	mx = int(v.Samples[0].R)
	for _, s := range v.Samples {
		mn = min(mn, min(int(s.R), min(int(s.G), int(s.B))))
		mx = max(mx, max(int(s.R), max(int(s.G), int(s.B))))
	}
	return
}

func (v *VLT) Rescale() {
	mn, mx := v.minmax()
	for i := range v.Samples {
		v.Samples[i] = rescaleSample(v.Samples[i], mn, mx)
	}
}

func (v VLT) sample(r, g, b int) Sample {
	idx := r + g*v.LUT3Dsize + b*v.LUT3Dsize*v.LUT3Dsize
	if idx >= len(v.Samples) {
		return Sample{}
	}
	return v.Samples[idx]
}

func (v VLT) interpolate(r, g, b float64) rgb {
	size := v.LUT3Dsize - 1
	sizeF := float64(size)

	rIdx := max(0.0, min(sizeF, r*sizeF))
	gIdx := max(0.0, min(sizeF, g*sizeF))
	bIdx := max(0.0, min(sizeF, b*sizeF))

	r0, g0, b0 := int(rIdx), int(gIdx), int(bIdx)
	r1, g1, b1 := min(r0+1, size), min(g0+1, size), min(b0+1, size)

	rFrac := rIdx - float64(r0)
	gFrac := gIdx - float64(g0)
	bFrac := bIdx - float64(b0)

	sf := func(r, g, b int) rgb { return toRGB(v.sample(r, g, b)) }

	return lerpRGB(
		lerpRGB(
			lerpRGB(sf(r0, g0, b0), sf(r1, g0, b0), rFrac),
			lerpRGB(sf(r0, g1, b0), sf(r1, g1, b0), rFrac),
			gFrac,
		),
		lerpRGB(
			lerpRGB(sf(r0, g0, b1), sf(r1, g0, b1), rFrac),
			lerpRGB(sf(r0, g1, b1), sf(r1, g1, b1), rFrac),
			gFrac,
		),
		bFrac,
	)
}

func (v VLT) Apply(img image.Image) *image.RGBA {
	return v.ApplyScaled(img, 1.0)
}

func (v VLT) ApplyScaled(img image.Image, intensity float64) *image.RGBA {
	bounds := img.Bounds()
	out := image.NewRGBA(bounds)

	intensity = max(0, min(1, intensity))

	var wg sync.WaitGroup
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		wg.Go(func() {
			v.applyRow(img, out, bounds, y, intensity)
		})
	}
	wg.Wait()
	return out
}

func (v VLT) applyRow(img image.Image, out *image.RGBA, bounds image.Rectangle, y int, intensity float64) {
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		r32, g32, b32, a32 := img.At(x, y).RGBA()

		r := float64(r32) / 65535.0
		g := float64(g32) / 65535.0
		b := float64(b32) / 65535.0

		s := v.interpolate(r, g, b)
		sr := s.r / maxVal
		sg := s.g / maxVal
		sb := s.b / maxVal

		br := max(0, min(1, r+(sr-r)*intensity))
		bg := max(0, min(1, g+(sg-g)*intensity))
		bb := max(0, min(1, b+(sb-b)*intensity))

		out.SetRGBA(x, y, color.RGBA{
			R: uint8(br * 255),
			G: uint8(bg * 255),
			B: uint8(bb * 255),
			A: uint8(a32 / 257),
		})
	}
}

func Load(r io.Reader) (VLT, error) {
	var (
		v       VLT
		scanner = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch {
		case fields[0] == "LUT_3D_SIZE":
			if _, err := fmt.Sscanf(line, "LUT_3D_SIZE %d", &v.LUT3Dsize); err != nil {
				return VLT{}, err
			}

		case len(fields) == 3:
			var rv, gv, bv int
			if _, err := fmt.Sscanf(line, "%d %d %d", &rv, &gv, &bv); err != nil {
				return VLT{}, err
			}
			v.Samples = append(v.Samples, Sample{
				R: uint16(rv),
				G: uint16(gv),
				B: uint16(bv),
			})

		default:
			return VLT{}, ErrUnrecognisedLine
		}
	}

	if err := scanner.Err(); err != nil {
		return v, err
	}

	return v, nil
}

func LoadFile(path string) (VLT, error) {
	f, err := os.Open(path)
	if err != nil {
		return VLT{}, err
	}
	defer f.Close()

	return Load(f)
}
