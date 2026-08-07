// Package lut provides a format-agnostic 3D LUT: a single lattice of float64
// samples that CUBE, HALD PNG and VLT files all convert to and from losslessly.
// Interpolation, application and the compose/invert/extract operations are
// implemented once here instead of once per file format.
package lut

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/NicoNex/prism/cube"
	"github.com/NicoNex/prism/hald"
	"github.com/NicoNex/prism/vlt"
)

// Sample is an RGB triple, nominally in [0, 1].
type Sample struct {
	R, G, B float64
}

// LUT is a 3D colour lookup table. Samples are indexed r + g*Size + b*Size²,
// i.e. red varies fastest, matching the CUBE and VLT file layouts.
type LUT struct {
	Title     string
	Size      int
	DomainMin Sample
	DomainMax Sample
	Samples   []Sample
}

// Interp selects the interpolation used when sampling between lattice nodes.
type Interp int

const (
	// Tetrahedral splits each lattice cell into 6 tetrahedra. It is the
	// default: unlike trilinear it reproduces the neutral axis exactly, so
	// greys stay neutral and log-to-display conversions do not pick up casts.
	Tetrahedral Interp = iota
	// Trilinear is the classic 8-corner interpolation.
	Trilinear
)

var (
	ErrTooSmall      = errors.New("LUT size must be at least 2")
	ErrEmpty         = errors.New("empty LUT")
	ErrSizeMismatch  = errors.New("images have different dimensions")
	ErrEmptyInput    = errors.New("no pixels to sample")
	ErrUnknownInterp = errors.New("unknown interpolation")
)

// ParseInterp maps a CLI name to an Interp.
func ParseInterp(s string) (Interp, error) {
	switch strings.ToLower(s) {
	case "tetra", "tetrahedral":
		return Tetrahedral, nil
	case "tri", "trilinear":
		return Trilinear, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrUnknownInterp, s)
	}
}

// New returns an identity LUT of the given lattice size.
func New(size int) *LUT {
	if size < 2 {
		size = 2
	}
	l := &LUT{
		Size:      size,
		DomainMin: Sample{0, 0, 0},
		DomainMax: Sample{1, 1, 1},
		Samples:   make([]Sample, size*size*size),
	}
	n := float64(size - 1)
	for b := range size {
		for g := range size {
			for r := range size {
				l.Samples[r+g*size+b*size*size] = Sample{
					R: float64(r) / n,
					G: float64(g) / n,
					B: float64(b) / n,
				}
			}
		}
	}
	return l
}

// Valid reports whether the lattice is consistent and usable.
func (l *LUT) Valid() error {
	if l == nil || len(l.Samples) == 0 {
		return ErrEmpty
	}
	if l.Size < 2 {
		return ErrTooSmall
	}
	if want := l.Size * l.Size * l.Size; len(l.Samples) != want {
		return fmt.Errorf("LUT_3D_SIZE %d needs %d samples, got %d", l.Size, want, len(l.Samples))
	}
	if l.DomainMax.R <= l.DomainMin.R ||
		l.DomainMax.G <= l.DomainMin.G ||
		l.DomainMax.B <= l.DomainMin.B {
		return cube.ErrInvalidDomain
	}
	return nil
}

func (l *LUT) at(r, g, b int) Sample {
	return l.Samples[r+g*l.Size+b*l.Size*l.Size]
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

// cell maps a domain value to the lower lattice index of its cell plus the
// fractional offset inside that cell.
func (l *LUT) cell(v, dmin, dmax float64) (int, float64) {
	n := l.Size - 1
	x := (v - dmin) / (dmax - dmin) * float64(n)
	x = math.Max(0, math.Min(float64(n), x))
	i := int(x)
	if i >= n {
		i = n - 1
	}
	return i, x - float64(i)
}

func sub(a, b Sample) Sample {
	return Sample{a.R - b.R, a.G - b.G, a.B - b.B}
}

// Tetrahedral samples the LUT by splitting the enclosing lattice cell into six
// tetrahedra and interpolating barycentrically within the one containing the
// point (Kasson et al.). The six cases below are the barycentric expansions of
// each tetrahedron, ordered by the ranking of the fractional coordinates.
func (l *LUT) Tetrahedral(r, g, b float64) Sample {
	ri, fr := l.cell(r, l.DomainMin.R, l.DomainMax.R)
	gi, fg := l.cell(g, l.DomainMin.G, l.DomainMax.G)
	bi, fb := l.cell(b, l.DomainMin.B, l.DomainMax.B)

	c := func(dr, dg, db int) Sample { return l.at(ri+dr, gi+dg, bi+db) }
	c000 := c(0, 0, 0)
	c111 := c(1, 1, 1)

	var (
		w1, w2, w3 float64
		d1, d2, d3 Sample
	)
	switch {
	case fr > fg && fg > fb: // fr > fg > fb
		w1, d1 = fr, sub(c(1, 0, 0), c000)
		w2, d2 = fg, sub(c(1, 1, 0), c(1, 0, 0))
		w3, d3 = fb, sub(c111, c(1, 1, 0))
	case fr > fg && fr > fb: // fr > fb >= fg
		w1, d1 = fr, sub(c(1, 0, 0), c000)
		w2, d2 = fb, sub(c(1, 0, 1), c(1, 0, 0))
		w3, d3 = fg, sub(c111, c(1, 0, 1))
	case fr > fg: // fb >= fr > fg
		w1, d1 = fb, sub(c(0, 0, 1), c000)
		w2, d2 = fr, sub(c(1, 0, 1), c(0, 0, 1))
		w3, d3 = fg, sub(c111, c(1, 0, 1))
	case fb > fg: // fb > fg >= fr
		w1, d1 = fb, sub(c(0, 0, 1), c000)
		w2, d2 = fg, sub(c(0, 1, 1), c(0, 0, 1))
		w3, d3 = fr, sub(c111, c(0, 1, 1))
	case fb > fr: // fg >= fb > fr
		w1, d1 = fg, sub(c(0, 1, 0), c000)
		w2, d2 = fb, sub(c(0, 1, 1), c(0, 1, 0))
		w3, d3 = fr, sub(c111, c(0, 1, 1))
	default: // fg >= fr >= fb
		w1, d1 = fg, sub(c(0, 1, 0), c000)
		w2, d2 = fr, sub(c(1, 1, 0), c(0, 1, 0))
		w3, d3 = fb, sub(c111, c(1, 1, 0))
	}

	return Sample{
		R: c000.R + w1*d1.R + w2*d2.R + w3*d3.R,
		G: c000.G + w1*d1.G + w2*d2.G + w3*d3.G,
		B: c000.B + w1*d1.B + w2*d2.B + w3*d3.B,
	}
}

func lerp(a, b Sample, t float64) Sample {
	return Sample{
		R: a.R + t*(b.R-a.R),
		G: a.G + t*(b.G-a.G),
		B: a.B + t*(b.B-a.B),
	}
}

// Trilinear samples the LUT with the classic 8-corner interpolation.
func (l *LUT) Trilinear(r, g, b float64) Sample {
	ri, fr := l.cell(r, l.DomainMin.R, l.DomainMax.R)
	gi, fg := l.cell(g, l.DomainMin.G, l.DomainMax.G)
	bi, fb := l.cell(b, l.DomainMin.B, l.DomainMax.B)

	c := func(dr, dg, db int) Sample { return l.at(ri+dr, gi+dg, bi+db) }

	return lerp(
		lerp(
			lerp(c(0, 0, 0), c(1, 0, 0), fr),
			lerp(c(0, 1, 0), c(1, 1, 0), fr),
			fg,
		),
		lerp(
			lerp(c(0, 0, 1), c(1, 0, 1), fr),
			lerp(c(0, 1, 1), c(1, 1, 1), fr),
			fg,
		),
		fb,
	)
}

// Eval samples the LUT with the requested interpolation.
func (l *LUT) Eval(r, g, b float64, in Interp) Sample {
	if in == Trilinear {
		return l.Trilinear(r, g, b)
	}
	return l.Tetrahedral(r, g, b)
}

func (l *LUT) evalFunc(in Interp) func(r, g, b float64) Sample {
	if in == Trilinear {
		return l.Trilinear
	}
	return l.Tetrahedral
}

// forEachSlice runs fn once per b-slice, spread over the available cores.
func forEachSlice(size int, fn func(b int)) {
	workers := min(runtime.NumCPU(), size)
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for b := w; b < size; b += workers {
				fn(b)
			}
		})
	}
	wg.Wait()
}

// straight reads a pixel as un-premultiplied RGB in [0, 1] plus its alpha.
func straight(c color.Color) (Sample, uint32) {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return Sample{}, 0
	}
	af := float64(a)
	return Sample{R: float64(r) / af, G: float64(g) / af, B: float64(b) / af}, a
}

// Apply maps every pixel of img through the LUT. intensity blends between the
// original and the mapped colour. Output is 16-bit to keep the precision that
// tetrahedral interpolation buys; 8-bit output would quantise it away.
func (l *LUT) Apply(img image.Image, intensity float64, in Interp) *image.NRGBA64 {
	bounds := img.Bounds()
	out := image.NewNRGBA64(bounds)
	intensity = clamp01(intensity)
	eval := l.evalFunc(in)

	rows := bounds.Dy()
	workers := min(runtime.NumCPU(), rows)
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for y := bounds.Min.Y + w; y < bounds.Max.Y; y += workers {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					s, a := straight(img.At(x, y))
					if a == 0 {
						continue // fully transparent: leave the zero pixel
					}
					m := eval(s.R, s.G, s.B)
					out.SetNRGBA64(x, y, color.NRGBA64{
						R: to16(s.R + (m.R-s.R)*intensity),
						G: to16(s.G + (m.G-s.G)*intensity),
						B: to16(s.B + (m.B-s.B)*intensity),
						A: uint16(a),
					})
				}
			}
		})
	}
	wg.Wait()
	return out
}

func to16(v float64) uint16 {
	return uint16(clamp01(v)*65535 + 0.5)
}

// Resample returns the LUT re-evaluated on a lattice of the given size with a
// [0, 1] domain. Growing the size never adds detail; shrinking it loses some.
func (l *LUT) Resample(size int) *LUT {
	if size < 2 {
		size = 2
	}
	if size == l.Size && l.DomainMin == (Sample{0, 0, 0}) && l.DomainMax == (Sample{1, 1, 1}) {
		out := &LUT{
			Title:     l.Title,
			Size:      l.Size,
			DomainMin: l.DomainMin,
			DomainMax: l.DomainMax,
			Samples:   make([]Sample, len(l.Samples)),
		}
		copy(out.Samples, l.Samples)
		return out
	}

	out := New(size)
	out.Title = l.Title
	n := float64(size - 1)
	dr := l.DomainMax.R - l.DomainMin.R
	dg := l.DomainMax.G - l.DomainMin.G
	db := l.DomainMax.B - l.DomainMin.B

	forEachSlice(size, func(bi int) {
		for gi := range size {
			for ri := range size {
				out.Samples[ri+gi*size+bi*size*size] = l.Tetrahedral(
					l.DomainMin.R+float64(ri)/n*dr,
					l.DomainMin.G+float64(gi)/n*dg,
					l.DomainMin.B+float64(bi)/n*db,
				)
			}
		}
	})
	return out
}

// identityAt returns the input colour that lattice index idx stands for.
func identityAt(idx, size int) Sample {
	n := float64(size - 1)
	r := idx % size
	g := (idx / size) % size
	b := idx / (size * size)
	return Sample{R: float64(r) / n, G: float64(g) / n, B: float64(b) / n}
}

// splat distributes val over the 8 lattice nodes surrounding pos, weighted
// trilinearly. It is the adjoint of trilinear interpolation. acc, w and w2
// accumulate weight·val, the weights, and the squared weights respectively;
// any of them may be nil.
func splat(acc []Sample, w, w2 []float64, size int, pos, val Sample) {
	n := size - 1
	nf := float64(n)

	idx := func(v float64) (int, float64) {
		x := clamp01(v) * nf
		i := int(x)
		if i >= n {
			i = n - 1
		}
		return i, x - float64(i)
	}

	r0, fr := idx(pos.R)
	g0, fg := idx(pos.G)
	b0, fb := idx(pos.B)

	for db := range 2 {
		wb := fb
		if db == 0 {
			wb = 1 - fb
		}
		for dg := range 2 {
			wg := fg
			if dg == 0 {
				wg = 1 - fg
			}
			for dr := range 2 {
				wr := fr
				if dr == 0 {
					wr = 1 - fr
				}
				wt := wr * wg * wb
				if wt == 0 {
					continue
				}
				i := (r0 + dr) + (g0+dg)*size + (b0+db)*size*size
				if w != nil {
					w[i] += wt
				}
				if w2 != nil {
					w2[i] += wt * wt
				}
				if acc != nil {
					acc[i].R += wt * val.R
					acc[i].G += wt * val.G
					acc[i].B += wt * val.B
				}
			}
		}
	}
}

// neighbours calls fn with the lattice index of each of the up-to-6 axis
// neighbours of idx.
func neighbours(idx, size int, fn func(n int)) {
	r := idx % size
	g := (idx / size) % size
	b := idx / (size * size)

	if r > 0 {
		fn(idx - 1)
	}
	if r < size-1 {
		fn(idx + 1)
	}
	if g > 0 {
		fn(idx - size)
	}
	if g < size-1 {
		fn(idx + size)
	}
	if b > 0 {
		fn(idx - size*size)
	}
	if b < size-1 {
		fn(idx + size*size)
	}
}

// ---- format conversion ----

// FromCube converts a parsed CUBE file to a LUT.
func FromCube(c cube.Cube) (*LUT, error) {
	l := &LUT{
		Title:     c.Title,
		Size:      c.LUT3Dsize,
		DomainMin: Sample(c.DomainMin),
		DomainMax: Sample(c.DomainMax),
		Samples:   make([]Sample, len(c.Samples)),
	}
	for i, s := range c.Samples {
		l.Samples[i] = Sample(s)
	}
	return l, l.Valid()
}

// ToCube converts a LUT to a CUBE file, resampling to size if it differs.
func ToCube(l *LUT, size int, title string) cube.Cube {
	src := l.Resample(size)
	c := cube.Cube{
		Title:     title,
		LUT3Dsize: src.Size,
		DomainMin: cube.Sample{R: 0, G: 0, B: 0},
		DomainMax: cube.Sample{R: 1, G: 1, B: 1},
		Samples:   make([]cube.Sample, len(src.Samples)),
	}
	if c.Title == "" {
		c.Title = l.Title
	}
	for i, s := range src.Samples {
		c.Samples[i] = cube.Sample(s)
	}
	return c
}

// FromVLT converts a parsed VLT file to a LUT. VLT samples are 12-bit.
func FromVLT(v vlt.VLT) (*LUT, error) {
	l := &LUT{
		Size:      v.LUT3Dsize,
		DomainMin: Sample{0, 0, 0},
		DomainMax: Sample{1, 1, 1},
		Samples:   make([]Sample, len(v.Samples)),
	}
	for i, s := range v.Samples {
		l.Samples[i] = Sample{
			R: float64(s.R) / 4095,
			G: float64(s.G) / 4095,
			B: float64(s.B) / 4095,
		}
	}
	return l, l.Valid()
}

// ToVLT converts a LUT to a VLT, resampling to size if it differs.
func ToVLT(l *LUT, size int) vlt.VLT {
	src := l.Resample(size)
	v := vlt.VLT{
		LUT3Dsize: src.Size,
		Samples:   make([]vlt.Sample, len(src.Samples)),
	}
	for i, s := range src.Samples {
		v.Samples[i] = vlt.Sample{
			R: uint16(clamp01(s.R)*4095 + 0.5),
			G: uint16(clamp01(s.G)*4095 + 0.5),
			B: uint16(clamp01(s.B)*4095 + 0.5),
		}
	}
	return v
}

// FromHALD converts a HALD CLUT image to a LUT. A HALD of level L carries an
// L²-per-axis lattice, so the conversion is exact.
func FromHALD(h hald.HALD) (*LUT, error) {
	level := h.Level()
	cells := level * level
	side := level * level * level
	origin := h.Bounds().Min

	l := &LUT{
		Size:      cells,
		DomainMin: Sample{0, 0, 0},
		DomainMax: Sample{1, 1, 1},
		Samples:   make([]Sample, cells*cells*cells),
	}
	for b := range cells {
		for g := range cells {
			for r := range cells {
				idx := b*cells*cells + g*cells + r
				s, _ := straight(h.At(origin.X+idx%side, origin.Y+idx/side))
				l.Samples[r+g*cells+b*cells*cells] = s
			}
		}
	}
	return l, l.Valid()
}

// ToHALD renders a LUT as a HALD CLUT image of the given level. The image is
// 16-bit: an 8-bit HALD quantises a LUT to 256 steps per channel and visibly
// bands log-to-display conversions.
func ToHALD(l *LUT, level int) image.Image {
	if level < 2 {
		level = 2
	}
	cells := level * level
	side := level * level * level
	src := l.Resample(cells)

	img := image.NewNRGBA64(image.Rect(0, 0, side, side))
	for b := range cells {
		for g := range cells {
			for r := range cells {
				s := src.Samples[r+g*cells+b*cells*cells]
				idx := b*cells*cells + g*cells + r
				img.SetNRGBA64(idx%side, idx/side, color.NRGBA64{
					R: to16(s.R),
					G: to16(s.G),
					B: to16(s.B),
					A: 65535,
				})
			}
		}
	}
	return img
}

// LoadFile reads a LUT from a .cube, .png (HALD) or .vlt file.
func LoadFile(path string) (*LUT, error) {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".cube":
		c, err := cube.LoadFile(path)
		if err != nil {
			return nil, err
		}
		return FromCube(c)

	case ".png":
		h, err := hald.LoadFile(path)
		if err != nil {
			return nil, err
		}
		return FromHALD(h)

	case ".vlt":
		v, err := vlt.LoadFile(path)
		if err != nil {
			return nil, err
		}
		return FromVLT(v)

	default:
		return nil, fmt.Errorf("unsupported LUT format: %q", ext)
	}
}

// defaultSize picks an output lattice size for a format when the user did not
// ask for one: keep the source size, unless it came from a HALD (which has a
// far denser lattice than any text format wants to carry).
func defaultSize(srcSize, maxSize, fallback int) int {
	if srcSize <= maxSize {
		return srcSize
	}
	return fallback
}

// SaveFile writes a LUT to a .cube, .png (HALD) or .vlt file. size selects the
// output lattice size (for .png it is the HALD level); 0 picks a sane default.
func SaveFile(path string, l *LUT, size int, title string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".cube":
		if size <= 0 {
			size = defaultSize(l.Size, 65, 33)
		}
		c := ToCube(l, size, title)
		if c.Title == "" {
			base := filepath.Base(path)
			c.Title = strings.TrimSuffix(base, filepath.Ext(base))
		}
		_, err = c.WriteTo(f)

	case ".vlt":
		if size <= 0 {
			size = defaultSize(l.Size, 33, 17)
		}
		_, err = ToVLT(l, size).WriteTo(f)

	case ".png":
		if size <= 0 {
			size = 12
		}
		err = png.Encode(f, ToHALD(l, size))

	default:
		err = fmt.Errorf("unsupported LUT format: %q", ext)
	}

	if err != nil {
		return err
	}
	return f.Close()
}
