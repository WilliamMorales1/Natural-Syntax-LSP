package main

import (
	"fmt"
	"math"
	"math/rand"
)

// Two fixed orthogonal unit vectors in embHiddenSize-space.
// Projecting a normalized embedding onto these gives (x,y); atan2(y,x) is the hue.
// Similar embeddings → nearby (x,y) → similar hue → similar color.
var (
	colorProjX [embHiddenSize]float32
	colorProjY [embHiddenSize]float32
)

func init() {
	rng := rand.New(rand.NewSource(0xC0105500))

	// First random unit vector.
	var norm float32
	for j := range embHiddenSize {
		x := float32(rng.NormFloat64())
		colorProjX[j] = x
		norm += x * x
	}
	norm = float32(math.Sqrt(float64(norm)))
	for j := range embHiddenSize {
		colorProjX[j] /= norm
	}

	// Second random vector, Gram-Schmidt orthogonalized against the first.
	var dot float32
	for j := range embHiddenSize {
		colorProjY[j] = float32(rng.NormFloat64())
	}
	for j := range embHiddenSize {
		dot += colorProjX[j] * colorProjY[j]
	}
	norm = 0
	for j := range embHiddenSize {
		colorProjY[j] -= dot * colorProjX[j]
		norm += colorProjY[j] * colorProjY[j]
	}
	norm = float32(math.Sqrt(float64(norm)))
	for j := range embHiddenSize {
		colorProjY[j] /= norm
	}
}

// embeddingToColor maps a normalized embedding to a "#RRGGBB" hex color.
// Projects onto a fixed 2D plane → angle → hue in [0°,360°] → HSL→RGB.
func embeddingToColor(v []float32) string {
	var px, py float32
	for j, vj := range v {
		px += vj * colorProjX[j]
		py += vj * colorProjY[j]
	}
	hue := math.Atan2(float64(py), float64(px))   // [-π, π]
	hue = (hue+math.Pi) / (2 * math.Pi) * 360     // [0°, 360°]
	r, g, b := hslToRGB(hue, 0.75, 0.62)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	if s == 0 {
		v := uint8(l * 255)
		return v, v, v
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	return uint8(hueToRGB(p, q, h/360+1.0/3.0) * 255),
		uint8(hueToRGB(p, q, h/360) * 255),
		uint8(hueToRGB(p, q, h/360-1.0/3.0) * 255)
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6*t
	case t < 0.5:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6
	default:
		return p
	}
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
