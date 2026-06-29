package main

import (
	"fmt"
	"math"
	"math/rand"
	"sync/atomic"
)

type semanticColorParams struct{ L, C float64 }

var semanticColorParamsPtr atomic.Pointer[semanticColorParams]

func init() {
	p := &semanticColorParams{L: 0.75, C: 0.14}
	semanticColorParamsPtr.Store(p)
}

func setSemanticColorParams(l, c float64) {
	p := &semanticColorParams{L: l, C: c}
	semanticColorParamsPtr.Store(p)
}

// Two fixed orthogonal unit vectors used to project embeddings to a 2D hue plane.
// Seeded deterministically so colors are stable across restarts.
var (
	colorProjX []float32
	colorProjY []float32
)

// initColorProjections builds the two projection vectors for the given hidden dim.
// Must be called once before any embeddingToColor call.
func initColorProjections(dim int) {
	rng := rand.New(rand.NewSource(0xC0105500))

	colorProjX = make([]float32, dim)
	colorProjY = make([]float32, dim)

	var norm float32
	for j := range dim {
		x := float32(rng.NormFloat64())
		colorProjX[j] = x
		norm += x * x
	}
	norm = float32(math.Sqrt(float64(norm)))
	for j := range dim {
		colorProjX[j] /= norm
	}

	var dot float32
	for j := range dim {
		colorProjY[j] = float32(rng.NormFloat64())
	}
	for j := range dim {
		dot += colorProjX[j] * colorProjY[j]
	}
	norm = 0
	for j := range dim {
		colorProjY[j] -= dot * colorProjX[j]
		norm += colorProjY[j] * colorProjY[j]
	}
	norm = float32(math.Sqrt(float64(norm)))
	for j := range dim {
		colorProjY[j] /= norm
	}
}

// embeddingToColor maps a normalized embedding to a "#RRGGBB" hex color using OKLCH.
// Projects onto a fixed 2D plane → angle → OKLCH hue → linear sRGB → gamma sRGB.
func embeddingToColor(v []float32) string {
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
func oklchToSRGB(L, C, hRad float64) (uint8, uint8, uint8) {
	a := C * math.Cos(hRad)
	b := C * math.Sin(hRad)

	// OKLab → linear sRGB (Björn Ottosson's matrix).
	l_ := L + 0.3963377774*a + 0.2158037573*b
	m_ := L - 0.1055613458*a - 0.0638541728*b
	s_ := L - 0.0894841775*a - 1.2914855480*b

	l := l_ * l_ * l_
	m := m_ * m_ * m_
	s := s_ * s_ * s_

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
	var g float64
	if c <= 0.0031308 {
		g = 12.92 * c
	} else {
		g = 1.055*math.Pow(c, 1.0/2.4) - 0.055
	}
	return uint8(g * 255)
}

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
