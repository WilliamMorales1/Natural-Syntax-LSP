package inference

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sync/atomic"
)

type semanticColorParams struct{ L, C float64 }

var semanticColorParamsPtr atomic.Pointer[semanticColorParams]

func init() {
	semanticColorParamsPtr.Store(&semanticColorParams{L: 0.75, C: 0.14})
}

// SetSemanticColorParams sets the OKLCH lightness/chroma used by EmbeddingToColor.
func SetSemanticColorParams(l, c float64) {
	semanticColorParamsPtr.Store(&semanticColorParams{L: l, C: c})
}

// SemanticColorParams returns the current OKLCH lightness/chroma.
func SemanticColorParams() (l, c float64) {
	p := semanticColorParamsPtr.Load()
	return p.L, p.C
}

// Two fixed orthogonal unit vectors, seeded deterministically, used to project embeddings to a 2D hue plane.
var (
	colorProjX []float32
	colorProjY []float32
)

// InitSemantic builds the two projection vectors for the given hidden dim; must be called once before any EmbeddingToColor call.
func InitSemantic(dim int) {
	rng := rand.New(rand.NewSource(0xC0105500))

	gaussian := func() []float32 {
		v := make([]float32, dim)
		for j := range v {
			v[j] = float32(rng.NormFloat64())
		}
		return v
	}
	colorProjX = l2Normalize(gaussian())
	// Gram-Schmidt: remove Y's X component so the two axes are orthogonal.
	y := gaussian()
	var dot float32
	for j := range y {
		dot += colorProjX[j] * y[j]
	}
	for j := range y {
		y[j] -= dot * colorProjX[j]
	}
	colorProjY = l2Normalize(y)
}

// EmbeddingToColor maps a normalized embedding to a "#RRGGBB" hex color: project onto a fixed 2D plane → angle → OKLCH hue → sRGB.
func EmbeddingToColor(v []float32) string {
	return hueToColor(dot(v, colorProjX), dot(v, colorProjY))
}

// Snapping keeps sub-visible refit drift from minting new hex values, since the client makes one decoration type per hex; a full step is ΔE ≈ 0.007 (hue, at C = 0.14) or 0.009 (chroma), under half an OKLab JND of 0.02.
const (
	hueSteps    = 120 // 3°
	chromaSteps = 16
)

// hueToColor converts the plane coordinates (px, py) to a full-chroma hex color whose OKLCH hue is their angle.
func hueToColor(px, py float32) string {
	p := semanticColorParamsPtr.Load()
	return oklchHex(p.L, p.C, snapHue(math.Atan2(float64(py), float64(px))))
}

// diskColor converts a point of the unit disk to a hex color: OKLCH hue from its angle, chroma scaled by its radius.
func diskColor(x, y float32) string {
	p := semanticColorParamsPtr.Load()
	r := math.Round(math.Hypot(float64(x), float64(y))*chromaSteps) / chromaSteps
	return oklchHex(p.L, p.C*r, snapHue(math.Atan2(float64(y), float64(x))))
}

func snapHue(hRad float64) float64 {
	const step = 2 * math.Pi / hueSteps
	return math.Round(hRad/step) * step
}

func oklchHex(lightness, chroma, hRad float64) string {
	r, g, b := oklchToSRGB(lightness, chroma, hRad)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

const (
	// planeMinWords is the document size below which a fitted plane is too noisy to beat the fixed one.
	planeMinWords = 50
	// planeColdIters bounds the first fit; planeWarmIters bounds refits, since a near-tie between the 2nd and 3rd axes can stall convergence while 3 warm iterations already land within ~0.01 ΔE/ΔE_max of exact.
	planeColdIters = 30
	planeWarmIters = 3
	// planeTol stops refitting once each new axis keeps all but this share of its length in the previous plane.
	planeTol = 1e-4
	// diskSamplePairs is how many word pairs fit the disk scale; 500 matched a 20k-pair fit within 0.001 mean error.
	diskSamplePairs = 500
	diskScaleSteps  = 80
	// diskScaleKeep keeps the previous scale unless the best grid scale beats it by this factor, so resampling pairs on each edit doesn't shift every word's chroma.
	diskScaleKeep = 0.98
)

// SemanticPlane is a per-document color plane: the top two principal axes of the document's embeddings, refit from its previous fit on each edit and rotated to match it so hues stay put.
// Each word's position in the plane, scaled into the unit disk, gives its color: angle is hue and radius is chroma, so close embeddings always get close colors.
type SemanticPlane struct {
	mean  []float32
	axes  [2][]float32 // nil until the first fit
	off   [2]float32   // mean's coordinates in the plane
	scale float32      // plane units → unit disk, whose rim is full chroma
}

// Fitted reports whether the plane has been fit; until then Color uses the fixed plane.
func (p *SemanticPlane) Fitted() bool { return p.axes[0] != nil }

// Fit refits the plane to unit embeddings by subspace iteration warm-started from the previous fit (or the fixed plane), then aligns it to that start; below planeMinWords it reverts to the fixed plane.
func (p *SemanticPlane) Fit(embeds [][]float32) {
	if len(embeds) < planeMinWords {
		*p = SemanticPlane{}
		return
	}
	prev, iters := p.axes, planeWarmIters
	if !p.Fitted() {
		prev, iters = [2][]float32{colorProjX, colorProjY}, planeColdIters
	}
	mean := meanOf(embeds)
	axes := [2][]float32{slices.Clone(prev[0]), slices.Clone(prev[1])}
	next := [2][]float32{make([]float32, len(mean)), make([]float32, len(mean))}
	for range iters {
		covMul(embeds, mean, axes, next)
		orthonormalize(next)
		converged := planeResidual(next[0], axes) < planeTol && planeResidual(next[1], axes) < planeTol
		axes, next = next, axes
		if converged {
			break
		}
	}
	alignPlane(prev, axes)
	p.mean, p.axes = mean, axes
	p.off = [2]float32{dot(mean, axes[0]), dot(mean, axes[1])}
	p.scale = p.fitScale(embeds)
}

// Color maps a unit embedding to a "#RRGGBB" hex color by its disk position in the fitted plane, or by its hue in the fixed plane before any fit.
func (p *SemanticPlane) Color(v []float32) string {
	if !p.Fitted() {
		return EmbeddingToColor(v)
	}
	return diskColor(p.diskPoint(v))
}

// coords returns v's centered position in the plane.
func (p *SemanticPlane) coords(v []float32) (x, y float32) {
	return dot(v, p.axes[0]) - p.off[0], dot(v, p.axes[1]) - p.off[1]
}

// diskPoint returns v's plane position times scale, clamped to the unit disk.
func (p *SemanticPlane) diskPoint(v []float32) (x, y float32) {
	x, y = p.coords(v)
	return clampUnit(x*p.scale, y*p.scale)
}

// fitScale picks the disk scale minimizing the mean gap between color and embedding chord distance over sampled word pairs, searching multiples of the 95th-percentile radius.
func (p *SemanticPlane) fitScale(embeds [][]float32) float32 {
	n := len(embeds)
	xs, ys, radii := make([]float32, n), make([]float32, n), make([]float32, n)
	for i, v := range embeds {
		xs[i], ys[i] = p.coords(v)
		radii[i] = float32(math.Hypot(float64(xs[i]), float64(ys[i])))
	}
	slices.Sort(radii)
	r95 := radii[n*95/100]
	if r95 < 1e-12 {
		return 1
	}
	type pair struct {
		i, j int
		dist float32
	}
	rng := rand.New(rand.NewSource(int64(n)))
	pairs := make([]pair, diskSamplePairs)
	for k := range pairs {
		i, j := rng.Intn(n), rng.Intn(n)
		// Unit embeddings: ‖v−w‖² = 2 − 2v·w.
		pairs[k] = pair{i, j, float32(math.Sqrt(max(0, float64(2-2*dot(embeds[i], embeds[j])))))}
	}
	gap := func(s float32) float32 {
		var sum float32
		for _, pr := range pairs {
			ax, ay := clampUnit(xs[pr.i]*s, ys[pr.i]*s)
			bx, by := clampUnit(xs[pr.j]*s, ys[pr.j]*s)
			sum += float32(math.Abs(math.Hypot(float64(ax-bx), float64(ay-by)) - float64(pr.dist)))
		}
		return sum
	}
	best, bestErr := 1/r95, float32(math.Inf(1))
	for k := 1; k < diskScaleSteps; k++ {
		s := float32(k) * 0.05 / r95
		if e := gap(s); e < bestErr {
			best, bestErr = s, e
		}
	}
	if p.scale > 0 && bestErr >= diskScaleKeep*gap(p.scale) {
		return p.scale
	}
	return best
}

// clampUnit pulls (x, y) radially onto the unit disk.
func clampUnit(x, y float32) (float32, float32) {
	r := float32(math.Hypot(float64(x), float64(y)))
	if r <= 1 {
		return x, y
	}
	return x / r, y / r
}

// covMul sets out[j] = Σ (x−mean)((x−mean)·vs[j]) for both vectors in one pass, without materializing the centered rows.
func covMul(xs [][]float32, mean []float32, vs, out [2][]float32) {
	var mv, sum [2]float32
	for j := range vs {
		mv[j] = dot(mean, vs[j])
		clear(out[j])
	}
	for _, x := range xs {
		for j := range vs {
			p := dot(x, vs[j]) - mv[j]
			sum[j] += p
			axpy(p, x, out[j])
		}
	}
	for j := range vs {
		axpy(-sum[j], mean, out[j])
	}
}

// orthonormalize applies Gram-Schmidt to the pair in place.
func orthonormalize(vs [2][]float32) {
	normalizeInPlace(vs[0])
	axpy(-dot(vs[1], vs[0]), vs[0], vs[1])
	normalizeInPlace(vs[1])
}

// planeResidual is the squared length of unit vector a outside the plane spanned by orthonormal vs.
func planeResidual(a []float32, vs [2][]float32) float32 {
	p0, p1 := dot(a, vs[0]), dot(a, vs[1])
	return 1 - p0*p0 - p1*p1
}

// alignPlane rotates or reflects axes within their plane to best match prev (2D orthogonal Procrustes), so a refit keeps hues where they were.
func alignPlane(prev, axes [2][]float32) {
	var m [2][2]float64
	for i := range 2 {
		for j := range 2 {
			m[i][j] = float64(dot(prev[i], axes[j]))
		}
	}
	// Best rotation and best reflection each reduce to one angle; keep whichever overlaps prev more.
	rotY, rotX := m[1][0]-m[0][1], m[0][0]+m[1][1]
	refY, refX := m[0][1]+m[1][0], m[0][0]-m[1][1]
	var r [2][2]float32
	if math.Hypot(rotY, rotX) >= math.Hypot(refY, refX) {
		th := math.Atan2(rotY, rotX)
		c, s := float32(math.Cos(th)), float32(math.Sin(th))
		r = [2][2]float32{{c, -s}, {s, c}}
	} else {
		th := math.Atan2(refY, refX)
		c, s := float32(math.Cos(th)), float32(math.Sin(th))
		r = [2][2]float32{{c, s}, {s, -c}}
	}
	for k := range axes[0] {
		a, b := axes[0][k], axes[1][k]
		axes[0][k] = r[0][0]*a + r[0][1]*b
		axes[1][k] = r[1][0]*a + r[1][1]*b
	}
}

func meanOf(xs [][]float32) []float32 {
	m := make([]float32, len(xs[0]))
	for _, x := range xs {
		axpy(1, x, m)
	}
	inv := 1 / float32(len(xs))
	for i := range m {
		m[i] *= inv
	}
	return m
}

func dot(a, b []float32) float32 {
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func axpy(alpha float32, x, y []float32) {
	for i := range x {
		y[i] += alpha * x[i]
	}
}

func normalizeInPlace(v []float32) {
	n := float32(math.Sqrt(float64(dot(v, v))))
	if n < 1e-12 {
		return
	}
	for i := range v {
		v[i] /= n
	}
}

// oklchToSRGB converts OKLCH (L, C, hue in radians) to gamma-corrected sRGB bytes.
func oklchToSRGB(lightness, chroma, hRad float64) (r, g, b uint8) {
	oa := chroma * math.Cos(hRad)
	ob := chroma * math.Sin(hRad)

	// OKLab → linear sRGB (Björn Ottosson's matrix); lc/mc/sc are the cube roots of LMS.
	lc := lightness + 0.3963377774*oa + 0.2158037573*ob
	mc := lightness - 0.1055613458*oa - 0.0638541728*ob
	sc := lightness - 0.0894841775*oa - 1.2914855480*ob

	l := lc * lc * lc
	m := mc * mc * mc
	s := sc * sc * sc

	rl := 4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	gl := -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	bl := -0.0041960863*l - 0.7034186147*m + 1.6956082452*s

	return linearToU8(rl), linearToU8(gl), linearToU8(bl)
}

// linearToU8 applies sRGB gamma and clamps to [0,255].
func linearToU8(c float64) uint8 {
	if c <= 0 {
		return 0
	}
	if c >= 1 {
		return 255
	}
	if c <= 0.0031308 {
		return uint8(12.92 * c * 255)
	}
	return uint8((1.055*math.Pow(c, 1.0/2.4) - 0.055) * 255)
}

// l2Normalize returns v scaled to unit length, or v itself if it is near zero.
func l2Normalize(v []float32) []float32 {
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	norm := float32(math.Sqrt(float64(sum)))
	if norm < 1e-8 {
		return v
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}
