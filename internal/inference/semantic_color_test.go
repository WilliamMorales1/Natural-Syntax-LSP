package inference

import (
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
)

const planeTestDim = 32

// clusteredRows draws n rows whose variance lies mostly along e0 and e1.
func clusteredRows(rng *rand.Rand, n int) [][]float32 {
	return noisyRows(rng, n, 0.2)
}

// noisyRows draws n unit rows with signal along e0 and e1 plus isotropic noise of the given scale.
func noisyRows(rng *rand.Rand, n int, noise float32) [][]float32 {
	rows := make([][]float32, n)
	for i := range rows {
		x := make([]float32, planeTestDim)
		for k := range x {
			x[k] = noise * float32(rng.NormFloat64())
		}
		x[0] += 3 * float32(rng.NormFloat64())
		x[1] += 2 * float32(rng.NormFloat64())
		rows[i] = l2Normalize(x)
	}
	return rows
}

func planeHue(p *SemanticPlane, v []float32) float64 {
	return math.Atan2(float64(dot(v, p.axes[1])-dot(p.mean, p.axes[1])), float64(dot(v, p.axes[0])-dot(p.mean, p.axes[0])))
}

// hueShift is the mean ΔE/ΔEmax between rows' hues under two fits.
func hueShift(a, b *SemanticPlane, rows [][]float32) float64 {
	var s float64
	for _, r := range rows {
		s += math.Abs(math.Sin((planeHue(a, r) - planeHue(b, r)) / 2))
	}
	return s / float64(len(rows))
}

func TestSemanticPlaneFindsPrincipalAxes(t *testing.T) {
	InitSemantic(planeTestDim)
	var p SemanticPlane
	p.Fit(clusteredRows(rand.New(rand.NewPCG(1, 2)), 300))
	if !p.Fitted() {
		t.Fatal("plane not fitted")
	}
	for k := range 2 {
		e := make([]float32, planeTestDim)
		e[k] = 1
		if r := planeResidual(e, p.axes); r > 0.01 {
			t.Errorf("e%d residual outside plane = %.4f", k, r)
		}
	}
}

func TestSemanticPlaneStableAcrossEdits(t *testing.T) {
	InitSemantic(planeTestDim)
	rng := rand.New(rand.NewPCG(3, 4))
	rows := clusteredRows(rng, 300)
	var p SemanticPlane
	p.Fit(rows)
	before := p

	p.Fit(rows)
	if s := hueShift(&before, &p, rows); s > 1e-3 {
		t.Errorf("refit on same rows moved hues by %.4f", s)
	}

	edited := append(rows[:len(rows):len(rows)], clusteredRows(rng, 25)...)
	p.Fit(edited)
	if s := hueShift(&before, &p, rows); s > 0.05 {
		t.Errorf("appending 25 rows moved old hues by %.4f", s)
	}
}

func TestSemanticPlaneSmallDocUsesFixedPlane(t *testing.T) {
	InitSemantic(planeTestDim)
	rows := clusteredRows(rand.New(rand.NewPCG(5, 6)), planeMinWords-1)
	var p SemanticPlane
	p.Fit(rows)
	if p.Fitted() {
		t.Fatal("fitted below planeMinWords")
	}
	for _, r := range rows {
		if got, want := p.Color(r), EmbeddingToColor(r); got != want {
			t.Fatalf("Color = %s, fixed plane gives %s", got, want)
		}
	}
}

func dist32(a, b []float32) float64 {
	var s float64
	for i := range a {
		d := float64(a[i] - b[i])
		s += d * d
	}
	return math.Sqrt(s)
}

func TestSemanticPlaneDiskLipschitz(t *testing.T) {
	InitSemantic(planeTestDim)
	rng := rand.New(rand.NewPCG(7, 8))
	rows := clusteredRows(rng, 300)
	var p SemanticPlane
	p.Fit(rows)
	if p.scale <= 0 {
		t.Fatalf("scale = %v", p.scale)
	}
	// Disk distance never exceeds scale × embedding distance, clamp included.
	for range 2000 {
		a, b := rows[rng.IntN(len(rows))], rows[rng.IntN(len(rows))]
		ax, ay := p.diskPoint(a)
		bx, by := p.diskPoint(b)
		if d, bound := math.Hypot(float64(ax-bx), float64(ay-by)), float64(p.scale)*dist32(a, b); d > bound+1e-5 {
			t.Fatalf("disk distance %.5f exceeds %.5f", d, bound)
		}
	}
	// Nearly identical embeddings get nearly identical colors, unlike the ring: usually the same snapped hex, else one snap step.
	same := 0
	for _, r := range rows[:50] {
		near := slices.Clone(r)
		for k := range near {
			near[k] += 1e-3 * float32(rng.NormFloat64())
		}
		near = l2Normalize(near)
		a, b := p.Color(r), p.Color(near)
		if a == b {
			same++
		}
		if got := hexDist(a, b); got > 12 {
			t.Errorf("perturbation moved color by %d per channel", got)
		}
	}
	if same < 40 {
		t.Errorf("only %d of 50 perturbations kept the same snapped color", same)
	}
}

func TestSemanticPlaneDiskScaleKeepsSpread(t *testing.T) {
	InitSemantic(planeTestDim)
	// Off-plane noise comparable to the signal, as in real embeddings; pure in-plane rows sit on a ring and rightly map to the rim.
	rows := noisyRows(rand.New(rand.NewPCG(9, 10)), 300, 1)
	var p SemanticPlane
	p.Fit(rows)
	onRim, colors := 0, map[string]bool{}
	for _, r := range rows {
		x, y := p.diskPoint(r)
		if math.Hypot(float64(x), float64(y)) > 0.999 {
			onRim++
		}
		colors[p.Color(r)] = true
	}
	// Real embeddings put 5-46% of words on the rim; the disk must keep a real interior, not collapse to the ring.
	if inside := len(rows) - onRim; inside < len(rows)/10 {
		t.Errorf("only %d of %d words inside the rim", inside, len(rows))
	}
	if len(colors) < len(rows)/4 {
		t.Errorf("%d distinct colors for %d words", len(colors), len(rows))
	}
}

// hexDist is the largest per-channel difference between two "#RRGGBB" colors.
func hexDist(a, b string) int {
	d := 0
	for i := 1; i < 7; i += 2 {
		x, _ := strconv.ParseUint(a[i:i+2], 16, 8)
		y, _ := strconv.ParseUint(b[i:i+2], 16, 8)
		d = max(d, int(x)-int(y), int(y)-int(x))
	}
	return d
}
