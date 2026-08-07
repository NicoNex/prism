package lut

import (
	"image"
	"math"
)

// MaxSolveSize caps the lattice Invert and Extract solve on. Both are iterative
// and cost O(size³) per sweep, and neither gains accuracy from a denser lattice
// than the data supports. Callers asking for more get a solve at this size that
// is then resampled up on write.
const MaxSolveSize = 65

func solveSize(size, fallback int) int {
	if size <= 0 {
		size = fallback
	}
	return max(2, min(size, MaxSolveSize))
}

// Compose returns the LUT equivalent to applying a and then b: out(x) = b(a(x)).
//
// Use it to bake a chain into a single file. Two common cases:
//
//	Compose(look, vlogToRec709)  // a look that outputs V-Log -> outputs Rec.709
//	Compose(rec709ToVlog, look)  // a look that expects V-Log -> expects Rec.709
//
// size is the output lattice size; 0 keeps the larger of the two inputs.
func Compose(a, b *LUT, size int) *LUT {
	if size <= 0 {
		// The composed function bends wherever either input bends, so it needs
		// a denser lattice than either to be represented without losing detail.
		size = min(2*max(a.Size, b.Size)-1, MaxSolveSize)
	}
	if size < 2 {
		size = 2
	}

	out := New(size)
	out.Title = a.Title
	n := float64(size - 1)

	forEachSlice(size, func(bi int) {
		for gi := range size {
			for ri := range size {
				s := a.Tetrahedral(float64(ri)/n, float64(gi)/n, float64(bi)/n)
				out.Samples[ri+gi*size+bi*size*size] = b.Tetrahedral(s.R, s.G, s.B)
			}
		}
	})
	return out
}

// Invert returns a numerical inverse of l: Invert(l)(l(x)) ≈ x.
//
// It works in three stages. First the source lattice is pushed forward through
// l and splatted into the output lattice, which seeds every node that the
// forward mapping actually reaches. Second, nodes outside the forward gamut are
// filled by diffusing their neighbours so the whole lattice starts from a
// plausible guess. Third, every node is refined with damped Gauss-Newton
// (Levenberg-Marquardt) against a finite-difference Jacobian of l, which is
// what makes the result accurate rather than merely smooth.
//
// A LUT that is not injective (clipped highlights, crushed blacks) has no true
// inverse there; those nodes settle on the closest preimage found.
func Invert(l *LUT, size int) *LUT {
	size = solveSize(size, defaultSize(l.Size, MaxSolveSize, MaxSolveSize))

	out := New(size)
	out.Title = l.Title
	nodes := size * size * size

	// Stage 1: forward-splat. Sampling l denser than the output lattice keeps
	// the seeds from getting holes wherever l compresses the range.
	acc := make([]Sample, nodes)
	wsum := make([]float64, nodes)
	dense := size * 2
	df := float64(dense - 1)
	for bi := range dense {
		for gi := range dense {
			for ri := range dense {
				p := Sample{float64(ri) / df, float64(gi) / df, float64(bi) / df}
				q := l.Tetrahedral(p.R, p.G, p.B)
				splat(acc, wsum, nil, size, q, p)
			}
		}
	}

	seeded := make([]bool, nodes)
	for i := range wsum {
		if wsum[i] > 1e-9 {
			seeded[i] = true
			out.Samples[i] = Sample{
				R: acc[i].R / wsum[i],
				G: acc[i].G / wsum[i],
				B: acc[i].B / wsum[i],
			}
		}
	}

	// Stage 2: diffuse the seeds into the unreached nodes.
	fillHoles(out, seeded)

	// Stage 3: per-node Levenberg-Marquardt refinement.
	// ponytail: one step is half a source cell; tetrahedral interpolation is
	// piecewise linear, so a finite-difference Jacobian is exact inside a cell.
	h := 0.5 / float64(l.Size-1)
	nf := float64(size - 1)
	forEachSlice(size, func(bi int) {
		for gi := range size {
			for ri := range size {
				i := ri + gi*size + bi*size*size
				y := Sample{float64(ri) / nf, float64(gi) / nf, float64(bi) / nf}
				out.Samples[i] = solvePreimage(l, y, out.Samples[i], h)
			}
		}
	})
	return out
}

// fillHoles replaces unseeded nodes with a smooth interpolation of their
// neighbours, leaving seeded nodes untouched.
func fillHoles(l *LUT, seeded []bool) {
	size := l.Size
	holes := 0
	for i, ok := range seeded {
		if !ok {
			l.Samples[i] = identityAt(i, size)
			holes++
		}
	}
	if holes == 0 {
		return
	}

	next := make([]Sample, len(l.Samples))
	copy(next, l.Samples)
	for range size * 2 {
		for i := range l.Samples {
			if seeded[i] {
				continue
			}
			var sum Sample
			var cnt float64
			neighbours(i, size, func(n int) {
				sum.R += l.Samples[n].R
				sum.G += l.Samples[n].G
				sum.B += l.Samples[n].B
				cnt++
			})
			next[i] = Sample{sum.R / cnt, sum.G / cnt, sum.B / cnt}
		}
		copy(l.Samples, next)
	}
}

func dist2(a, b Sample) float64 {
	dr, dg, db := a.R-b.R, a.G-b.G, a.B-b.B
	return dr*dr + dg*dg + db*db
}

func clampS(s Sample) Sample {
	return Sample{clamp01(s.R), clamp01(s.G), clamp01(s.B)}
}

func axis(s Sample, i int) float64 {
	switch i {
	case 0:
		return s.R
	case 1:
		return s.G
	default:
		return s.B
	}
}

func withAxis(s Sample, i int, v float64) Sample {
	switch i {
	case 0:
		s.R = v
	case 1:
		s.G = v
	default:
		s.B = v
	}
	return s
}

// partial estimates the derivative of l along one input axis at x, using a
// central difference where there is room and a one-sided one at the domain edge.
func partial(l *LUT, x Sample, i int, h float64) Sample {
	lo := math.Max(0, axis(x, i)-h)
	hi := math.Min(1, axis(x, i)+h)
	d := hi - lo
	if d < 1e-12 {
		return Sample{}
	}
	a := withAxis(x, i, lo)
	b := withAxis(x, i, hi)
	fa := l.Tetrahedral(a.R, a.G, a.B)
	fb := l.Tetrahedral(b.R, b.G, b.B)
	return Sample{(fb.R - fa.R) / d, (fb.G - fa.G) / d, (fb.B - fa.B) / d}
}

// solvePreimage finds x with l(x) ≈ y, starting from x0, by damped Gauss-Newton.
func solvePreimage(l *LUT, y, x0 Sample, h float64) Sample {
	x := clampS(x0)
	best := x
	bestErr := dist2(l.Tetrahedral(x.R, x.G, x.B), y)
	lambda := 1e-4

	for range 40 {
		if bestErr < 1e-14 {
			break
		}
		f := l.Tetrahedral(x.R, x.G, x.B)
		e := sub(y, f)

		j0 := partial(l, x, 0, h)
		j1 := partial(l, x, 1, h)
		j2 := partial(l, x, 2, h)

		// Normal equations: (JᵀJ + λ·diag(JᵀJ)) dx = Jᵀe
		var a [3][3]float64
		cols := [3]Sample{j0, j1, j2}
		for i := range 3 {
			for k := range 3 {
				ci, ck := cols[i], cols[k]
				a[i][k] = ci.R*ck.R + ci.G*ck.G + ci.B*ck.B
			}
		}
		g := [3]float64{
			j0.R*e.R + j0.G*e.G + j0.B*e.B,
			j1.R*e.R + j1.G*e.G + j1.B*e.B,
			j2.R*e.R + j2.G*e.G + j2.B*e.B,
		}
		for i := range 3 {
			a[i][i] += lambda*a[i][i] + 1e-12
		}

		dx, ok := solve3(a, g)
		if !ok {
			break
		}

		cand := clampS(Sample{x.R + dx[0], x.G + dx[1], x.B + dx[2]})
		candErr := dist2(l.Tetrahedral(cand.R, cand.G, cand.B), y)
		if candErr < bestErr {
			best, bestErr, x = cand, candErr, cand
			lambda = math.Max(lambda/3, 1e-9)
		} else {
			lambda *= 4
			if lambda > 1e6 {
				break
			}
		}
	}
	return best
}

// solve3 solves a 3x3 system by Cramer's rule.
func solve3(a [3][3]float64, b [3]float64) ([3]float64, bool) {
	det := a[0][0]*(a[1][1]*a[2][2]-a[1][2]*a[2][1]) -
		a[0][1]*(a[1][0]*a[2][2]-a[1][2]*a[2][0]) +
		a[0][2]*(a[1][0]*a[2][1]-a[1][1]*a[2][0])
	if math.Abs(det) < 1e-18 {
		return [3]float64{}, false
	}

	var out [3]float64
	for c := range 3 {
		m := a
		for r := range 3 {
			m[r][c] = b[r]
		}
		d := m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
			m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
			m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
		out[c] = d / det
	}
	return out, true
}

// DefaultSmoothing is the regularisation weight Extract uses when none is given.
// The data term is normalised per node, so a well-covered node carries a weight
// near 1; this leaves the measurement clearly in charge where there is data.
const DefaultSmoothing = 0.05

// Extract recovers the LUT that best maps src onto dst, given a matched pair of
// images (the same frame before and after a grade).
//
// A pair of images only constrains the colours it actually contains, so this is
// a scattered-data fit, not a lookup. It minimises
//
//	‖A·L − graded‖² + smoothing·‖∇L‖² + mu·‖L − identity‖²
//
// where A is trilinear interpolation of the lattice at each source pixel. Well
// covered colours follow the data, sparse ones follow their neighbours, and
// colours absent from the image stay at identity — "leave unchanged" — rather
// than drifting to whatever the sparse data extrapolates.
//
// The normal equations are built exactly rather than approximated. Every pixel
// touches the 8 corners of one lattice cell, so AᵀA is nonzero only between
// nodes at most one step apart on each axis: it fits in a 27-point stencil that
// one pass over the pixels fills in. Merely splatting each pixel onto the
// lattice and dividing by the accumulated weight — the cheap alternative — fits
// the local *mean* of the graded pixels around each node instead of the value
// at the node, which visibly biases the result wherever the grade is curved.
//
// smoothing <= 0 uses DefaultSmoothing. Raise it for noisy or compressed
// sources, lower it when the pair covers the gamut densely and cleanly.
func Extract(src, dst image.Image, size int, smoothing float64) (*LUT, error) {
	sb, db := src.Bounds(), dst.Bounds()
	if sb.Dx() != db.Dx() || sb.Dy() != db.Dy() {
		return nil, ErrSizeMismatch
	}
	size = solveSize(size, 33)
	if smoothing <= 0 {
		smoothing = DefaultSmoothing
	}

	const (
		stride = 27 // 3x3x3 stencil
		self   = 13 // offset (0,0,0) within it
		iters  = 400
		omega  = 1.6 // over-relaxation
		eps    = 1e-11
	)

	// The pull towards identity is what makes unmeasured colours decay to "leave
	// unchanged". Tying it to smoothing fixes how far a measured colour reaches
	// into empty parts of the cube — sqrt(smoothing/mu), about 10 lattice steps —
	// so `smoothing` stays a pure fit-versus-smooth knob and does not also
	// silently change how far a grade spreads.
	mu := smoothing / 100

	nodes := size * size * size
	m := make([]float64, nodes*stride)
	rhs := make([]Sample, nodes)
	var total float64

	n := size - 1
	nf := float64(n)
	corner := func(v float64) (int, float64) {
		x := clamp01(v) * nf
		i := int(x)
		if i >= n {
			i = n - 1
		}
		return i, x - float64(i)
	}

	var (
		wgt [8]float64
		idx [8]int
		crn [8][3]int
	)

	for y := range sb.Dy() {
		for x := range sb.Dx() {
			s, sa := straight(src.At(sb.Min.X+x, sb.Min.Y+y))
			d, da := straight(dst.At(db.Min.X+x, db.Min.Y+y))
			if sa == 0 || da == 0 {
				continue // transparent pixels carry no colour evidence
			}
			total++

			r0, fr := corner(s.R)
			g0, fg := corner(s.G)
			b0, fb := corner(s.B)

			c := 0
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
						wgt[c] = wr * wg * wb
						idx[c] = (r0 + dr) + (g0+dg)*size + (b0+db)*size*size
						crn[c] = [3]int{dr, dg, db}
						c++
					}
				}
			}

			for a := range 8 {
				wa := wgt[a]
				if wa == 0 {
					continue
				}
				// Fit the colour *change*, not the colour: with x = identity + D the
				// data residual is (graded - source), because trilinear interpolation
				// of the identity lattice reproduces the source exactly.
				rhs[idx[a]].R += wa * (d.R - s.R)
				rhs[idx[a]].G += wa * (d.G - s.G)
				rhs[idx[a]].B += wa * (d.B - s.B)

				base := idx[a] * stride
				for b := range 8 {
					if wgt[b] == 0 {
						continue
					}
					off := (crn[b][2]-crn[a][2]+1)*9 + (crn[b][1]-crn[a][1]+1)*3 + (crn[b][0] - crn[a][0] + 1)
					m[base+off] += wa * wgt[b]
				}
			}
		}
	}
	if total == 0 {
		return nil, ErrEmptyInput
	}

	// Normalise the data term by pixels-per-node so that a given `smoothing`
	// means the same thing whatever the image resolution.
	scale := float64(nodes) / total
	for i := range m {
		m[i] *= scale
	}
	for i := range rhs {
		rhs[i].R *= scale
		rhs[i].G *= scale
		rhs[i].B *= scale
	}

	// Node offset for each stencil position.
	var deltas [stride]int
	for dz := -1; dz <= 1; dz++ {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				deltas[(dz+1)*9+(dy+1)*3+(dx+1)] = dx + dy*size + dz*size*size
			}
		}
	}

	// Gauss-Seidel with over-relaxation on (AᵀA + smoothing·Laplacian + mu·I),
	// solving for the residual D = LUT - identity. Smoothing the residual rather
	// than the LUT itself matters: ‖∇LUT‖² is minimised by a *constant* LUT, so
	// regularising the LUT directly would flatten the whole cube to one colour.
	// ‖∇D‖² is minimised by leaving the image alone, which is the right default.
	d := make([]Sample, nodes)
	for range iters {
		var worst float64
		for i := range d {
			base := i * stride
			var coupled Sample
			for o := range stride {
				v := m[base+o]
				if v == 0 || o == self {
					continue
				}
				j := i + deltas[o]
				coupled.R += v * d[j].R
				coupled.G += v * d[j].G
				coupled.B += v * d[j].B
			}

			var sum Sample
			var deg float64
			neighbours(i, size, func(nb int) {
				sum.R += d[nb].R
				sum.G += d[nb].G
				sum.B += d[nb].B
				deg++
			})

			den := m[base+self] + smoothing*deg + mu
			cur := d[i]
			next := Sample{
				R: cur.R + omega*((rhs[i].R+smoothing*sum.R-coupled.R)/den-cur.R),
				G: cur.G + omega*((rhs[i].G+smoothing*sum.G-coupled.G)/den-cur.G),
				B: cur.B + omega*((rhs[i].B+smoothing*sum.B-coupled.B)/den-cur.B),
			}
			worst = math.Max(worst, dist2(next, cur))
			d[i] = next
		}
		if math.Sqrt(worst) < eps {
			break
		}
	}

	out := New(size)
	for i := range out.Samples {
		out.Samples[i] = clampS(Sample{
			R: out.Samples[i].R + d[i].R,
			G: out.Samples[i].G + d[i].G,
			B: out.Samples[i].B + d[i].B,
		})
	}
	return out, nil
}
