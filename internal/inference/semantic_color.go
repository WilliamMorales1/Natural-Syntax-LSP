package inference

import (
	"fmt"
	"math"
	"math/rand"
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
	var px, py float32
	for j, vj := range v {
		px += vj * colorProjX[j]
		py += vj * colorProjY[j]
	}
	hRad := math.Atan2(float64(py), float64(px)) // [-π, π]
	p := semanticColorParamsPtr.Load()
	r, g, b := oklchToSRGB(p.L, p.C, hRad)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
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
