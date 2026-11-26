package render

type offsetTap struct {
	off    int
	weight float64
}

// Dither turns luminance into dots. Unknown algorithm names fall back to
// atkinson; callers that expose the value to users should validate it with
// ParseAlgorithm first.
func Dither(lum []float64, dots []bool, workBuf []float64, w, h int, algorithm string, density float64, prevDots []bool, stability float64) {
	name, ok := ParseAlgorithm(algorithm)
	if !ok {
		name = "atkinson"
	}

	switch name {
	case "bayer4":
		ditherOrdered(lum, dots, w, h, bayer4, 4, density, prevDots, stability)
	case "bayer8":
		ditherOrdered(lum, dots, w, h, bayer8, 8, density, prevDots, stability)
	case "bayer16":
		ditherOrdered(lum, dots, w, h, bayer16, 16, density, prevDots, stability)
	default:
		kernel := diffusionKernels[name]
		if kernel == nil {
			kernel = diffusionKernels["atkinson"]
		}
		ditherDiffusion(lum, dots, workBuf, w, h, kernel, density, prevDots, stability)
	}
}

func ditherOrdered(lum []float64, dots []bool, w, h int, matrix []float64, n int, density float64, prevDots []bool, stability float64) {
	hysteresis := stability * 0.05
	hasPrev := len(prevDots) == w*h && stability > 0

	for y := 0; y < h; y++ {
		row := y * w
		mrow := (y % n) * n
		bx := 0
		for x := 0; x < w; x++ {
			idx := row + x
			l := lum[idx]
			if l <= 0 {
				dots[idx] = false
			} else {
				threshold := matrix[mrow+bx]
				if hasPrev {
					if prevDots[idx] {
						threshold -= hysteresis
					} else {
						threshold += hysteresis
					}
				}
				dots[idx] = (l * density) > threshold
			}
			bx++
			if bx == n {
				bx = 0
			}
		}
	}
}

func ditherDiffusion(lum []float64, dots []bool, workBuf []float64, w, h int, kernel *diffusionKernel, density float64, prevDots []bool, stability float64) {
	total := w * h
	if len(workBuf) < total {
		workBuf = make([]float64, total)
	}
	buf := workBuf[:total]
	for i := 0; i < total; i++ {
		buf[i] = lum[i] * density
	}

	stickiness := stability * 0.25
	hasPrev := len(prevDots) == total && stability > 0

	// Neighbor offsets depend on width, so they are built once per call. Two
	// sets cover the left-to-right and right-to-left passes.
	var fwd, bwd [12]offsetTap
	n := len(kernel.taps)
	for i, t := range kernel.taps {
		fwd[i] = offsetTap{off: t.dy*w + t.dx, weight: t.weight}
		bwd[i] = offsetTap{off: t.dy*w - t.dx, weight: t.weight}
	}

	for y := 0; y < h; y++ {
		row := y * w
		start, end, step := 0, w, 1
		taps := fwd[:n]
		dir := 1
		if y&1 == 1 {
			start, end, step = w-1, -1, -1
			taps = bwd[:n]
			dir = -1
		}
		allRows := y+kernel.maxDY < h

		for x := start; x != end; x += step {
			idx := row + x
			val := buf[idx]
			threshold := 0.5
			if hasPrev {
				if prevDots[idx] {
					threshold -= stickiness
				} else {
					threshold += stickiness
				}
			}

			newVal := 0.0
			if val >= threshold {
				newVal = 1.0
				dots[idx] = true
			} else {
				dots[idx] = false
			}
			err := val - newVal

			if allRows && x >= kernel.maxDX && x <= w-1-kernel.maxDX {
				for i := 0; i < n; i++ {
					buf[idx+taps[i].off] += err * taps[i].weight
				}
			} else {
				for _, t := range kernel.taps {
					nx := x + dir*t.dx
					if nx < 0 || nx >= w || y+t.dy >= h {
						continue
					}
					buf[idx+t.dy*w+dir*t.dx] += err * t.weight
				}
			}
		}
	}
}
