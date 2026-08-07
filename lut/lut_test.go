package lut

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"testing"

	"github.com/NicoNex/prism/hald"
)

// vlogish is a stand-in for a log-to-display transfer: monotonic, strongly
// curved, and channel-crossing enough to exercise the 3D machinery. The matrix
// rows are positive and sum to 1, so it never leaves [0, 1] and stays
// invertible — a clipping transform has no inverse to test against.
func vlogish(s Sample) Sample {
	tone := func(v float64) float64 {
		return math.Pow(clamp01(v), 1/2.4)
	}
	r, g, b := tone(s.R), tone(s.G), tone(s.B)
	return Sample{
		R: clamp01(0.94*r + 0.04*g + 0.02*b),
		G: clamp01(0.03*r + 0.95*g + 0.02*b),
		B: clamp01(0.02*r + 0.03*g + 0.95*b),
	}
}

// haldOf round-trips an image through PNG so the hald package parses it the
// same way it would a file on disk.
func haldOf(img image.Image) (hald.HALD, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return hald.HALD{}, err
	}
	return hald.Load(&buf)
}

// build makes a LUT of the given size from a colour function.
func build(size int, fn func(Sample) Sample) *LUT {
	l := New(size)
	for i := range l.Samples {
		l.Samples[i] = fn(identityAt(i, size))
	}
	return l
}

func maxDiff(a, b Sample) float64 {
	return math.Max(math.Abs(a.R-b.R), math.Max(math.Abs(a.G-b.G), math.Abs(a.B-b.B)))
}

// Tetrahedral must reproduce lattice nodes exactly and stay inside the cell's
// range everywhere else.
func TestTetrahedralHitsNodes(t *testing.T) {
	const size = 9
	l := build(size, vlogish)

	for i := range l.Samples {
		p := identityAt(i, size)
		got := l.Tetrahedral(p.R, p.G, p.B)
		if d := maxDiff(got, l.Samples[i]); d > 1e-12 {
			t.Fatalf("node %d: tetrahedral off by %g", i, d)
		}
	}

	// Both interpolators must agree at nodes and stay close between them.
	for range 500 {
		r, g, b := rand.Float64(), rand.Float64(), rand.Float64()
		tet := l.Tetrahedral(r, g, b)
		tri := l.Trilinear(r, g, b)
		if d := maxDiff(tet, tri); d > 0.05 {
			t.Fatalf("tetra and tri disagree by %g at (%g %g %g)", d, r, g, b)
		}
	}
}

// Tetrahedral interpolation of a neutral input must stay neutral. This is the
// property trilinear loses, and the reason it is the default.
func TestTetrahedralPreservesNeutrals(t *testing.T) {
	// A LUT that is exactly neutral-preserving but far from linear off-axis.
	l := build(5, func(s Sample) Sample {
		return Sample{
			R: clamp01(s.R + 0.30*(s.G-s.B)),
			G: clamp01(s.G + 0.30*(s.B-s.R)),
			B: clamp01(s.B + 0.30*(s.R-s.G)),
		}
	})

	for i := range 200 {
		v := float64(i) / 199
		got := l.Tetrahedral(v, v, v)
		if d := maxDiff(got, Sample{v, v, v}); d > 1e-12 {
			t.Fatalf("grey %g drifted by %g (tetrahedral must preserve neutrals)", v, d)
		}
	}
}

func TestComposeWithIdentity(t *testing.T) {
	l := build(17, vlogish)
	got := Compose(l, New(2), 17)

	for i := range l.Samples {
		if d := maxDiff(got.Samples[i], l.Samples[i]); d > 1e-9 {
			t.Fatalf("compose with identity changed node %d by %g", i, d)
		}
	}
}

func TestComposeOrder(t *testing.T) {
	a := build(9, func(s Sample) Sample { return Sample{clamp01(s.R * 0.5), s.G, s.B} })
	b := build(9, func(s Sample) Sample { return Sample{clamp01(s.R + 0.25), s.G, s.B} })

	ab := Compose(a, b, 17) // b(a(x)): halve, then add
	got := ab.Tetrahedral(1, 0.5, 0.5)
	if math.Abs(got.R-0.75) > 1e-6 {
		t.Fatalf("compose(a,b) should give 1*0.5+0.25 = 0.75, got %g", got.R)
	}

	ba := Compose(b, a, 17) // a(b(x)): add, then halve
	got = ba.Tetrahedral(1, 0.5, 0.5)
	if math.Abs(got.R-0.5) > 1e-6 {
		t.Fatalf("compose(b,a) should give min(1,1.25)*0.5 = 0.5, got %g", got.R)
	}
}

// Invert must actually undo the LUT, not merely look plausible.
func TestInvertRoundTrip(t *testing.T) {
	fwd := build(33, vlogish)
	inv := Invert(fwd, 33)

	var worst float64
	for range 3000 {
		// Stay off the very edges: a LUT that clips has no inverse there.
		p := Sample{
			R: 0.02 + rand.Float64()*0.96,
			G: 0.02 + rand.Float64()*0.96,
			B: 0.02 + rand.Float64()*0.96,
		}
		q := fwd.Tetrahedral(p.R, p.G, p.B)
		back := inv.Tetrahedral(q.R, q.G, q.B)
		worst = math.Max(worst, maxDiff(back, p))
	}
	if worst > 0.02 {
		t.Fatalf("invert round-trip error %g, want <= 0.02", worst)
	}

	// Composing a LUT with its inverse must be near-identity.
	id := Compose(fwd, inv, 17)
	worst = 0
	for i := range id.Samples {
		worst = math.Max(worst, maxDiff(id.Samples[i], identityAt(i, 17)))
	}
	if worst > 0.05 {
		t.Fatalf("compose(fwd, invert(fwd)) deviates from identity by %g", worst)
	}
}

// Extract must recover a known LUT from an identity HALD graded through it.
func TestExtractFromIdentityHALD(t *testing.T) {
	const size = 33
	want := build(size, vlogish)

	// Level 6 is a 36-per-axis lattice: denser than the 33 we solve for, so
	// every node of the output has pixels landing on it.
	src := ToHALD(New(2), 6)
	graded := want.Apply(src, 1, Tetrahedral)

	got, err := Extract(src, graded, size, 0) // default smoothing
	if err != nil {
		t.Fatal(err)
	}

	var worst float64
	for range 3000 {
		p := Sample{rand.Float64(), rand.Float64(), rand.Float64()}
		worst = math.Max(worst, maxDiff(
			got.Tetrahedral(p.R, p.G, p.B),
			want.Tetrahedral(p.R, p.G, p.B),
		))
	}
	if worst > 0.02 {
		t.Fatalf("extracted LUT differs from the real one by %g, want <= 0.02", worst)
	}
}

// Colours absent from the image pair must decay towards "leave unchanged"
// rather than towards whatever the sparse data extrapolates to.
func TestExtractLeavesUnseenColoursAlone(t *testing.T) {
	// Solid mid-grey lifted to 0.75: exactly one colour in the cube is
	// constrained, and it is constrained to move a long way.
	src := image.NewNRGBA64(image.Rect(0, 0, 16, 16))
	dst := image.NewNRGBA64(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			src.SetNRGBA64(x, y, color.NRGBA64{R: 32768, G: 32768, B: 32768, A: 65535})
			dst.SetNRGBA64(x, y, color.NRGBA64{R: 49151, G: 49151, B: 49151, A: 65535})
		}
	}

	got, err := Extract(src, dst, 17, 0)
	if err != nil {
		t.Fatal(err)
	}

	// The measured colour must follow the data.
	if s := got.Tetrahedral(0.5, 0.5, 0.5); maxDiff(s, Sample{0.75, 0.75, 0.75}) > 0.05 {
		t.Fatalf("measured colour landed at %+v, want ~0.75 grey", s)
	}
	// Saturated red is nowhere in the input, so it must come back near untouched.
	if s := got.Tetrahedral(1, 0, 0); maxDiff(s, Sample{1, 0, 0}) > 0.05 {
		t.Fatalf("unconstrained colour moved to %+v, want it left near identity", s)
	}
}

func TestExtractRejectsMismatchedSizes(t *testing.T) {
	a := image.NewNRGBA64(image.Rect(0, 0, 4, 4))
	b := image.NewNRGBA64(image.Rect(0, 0, 5, 4))
	if _, err := Extract(a, b, 9, 0); err != ErrSizeMismatch {
		t.Fatalf("got %v, want ErrSizeMismatch", err)
	}
}

// Every format must survive a round trip through the canonical LUT.
func TestFormatRoundTrips(t *testing.T) {
	src := build(17, vlogish)

	t.Run("cube", func(t *testing.T) {
		got, err := FromCube(ToCube(src, 17, "t"))
		if err != nil {
			t.Fatal(err)
		}
		assertClose(t, src, got, 1e-12)
	})

	t.Run("vlt", func(t *testing.T) {
		got, err := FromVLT(ToVLT(src, 17))
		if err != nil {
			t.Fatal(err)
		}
		assertClose(t, src, got, 1.0/4095) // 12-bit quantisation
	})

	t.Run("hald", func(t *testing.T) {
		// A HALD of level L carries an L²-per-axis lattice, so a 25-node LUT
		// round-trips through level 5 with no resampling at all. Only 16-bit
		// storage quantisation should survive.
		l25 := build(25, vlogish)
		h, err := haldOf(ToHALD(l25, 5))
		if err != nil {
			t.Fatal(err)
		}
		got, err := FromHALD(h)
		if err != nil {
			t.Fatal(err)
		}
		assertClose(t, l25, got, 2.0/65535)
	})
}

func assertClose(t *testing.T, want, got *LUT, tol float64) {
	t.Helper()
	if got.Size != want.Size {
		t.Fatalf("size %d, want %d", got.Size, want.Size)
	}
	for i := range want.Samples {
		if d := maxDiff(got.Samples[i], want.Samples[i]); d > tol {
			t.Fatalf("node %d off by %g (tolerance %g)", i, d, tol)
		}
	}
}

func TestApplyIdentityIsLossless(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			src.SetNRGBA64(x, y, color.NRGBA64{
				R: uint16(x * 2114),
				G: uint16(y * 2114),
				B: uint16((x + y) * 1057),
				A: 65535,
			})
		}
	}

	out := New(2).Apply(src, 1, Tetrahedral)
	for y := range 32 {
		for x := range 32 {
			w := src.NRGBA64At(x, y)
			g := out.NRGBA64At(x, y)
			if absDiff(w.R, g.R) > 1 || absDiff(w.G, g.G) > 1 || absDiff(w.B, g.B) > 1 {
				t.Fatalf("identity LUT changed pixel (%d,%d): %v -> %v", x, y, w, g)
			}
		}
	}
}

func absDiff(a, b uint16) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
