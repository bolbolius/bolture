package render

import (
	"image"
	"image/color"
	"math"
)

// ShadowCutoff is the luminance floor below which pixels produce no dots.
const ShadowCutoff = 0.05

type Options struct {
	Width             int
	Height            int
	DotSize           float64
	Density           float64
	Dither            string
	TemporalStability float64
	Foreground        color.RGBA
	Background        color.RGBA
}

type Renderer struct {
	opts          Options
	step          int
	lumBuf        []float64
	ditherBuf     []float64
	dotsBuf       []bool
	prevLum       []float64
	prevDots      []bool
	hasPrevFrame  bool
	gridLum       []float64
	gridDitherBuf []float64
	gridDotsBuf   []bool
	prevGridLum   []float64
	prevGridDots  []bool
	hasPrevGrid   bool
}

func NewRenderer(opts Options) *Renderer {
	if opts.Width <= 0 {
		opts.Width = 1280
	}
	if opts.Height <= 0 {
		opts.Height = 720
	}
	if opts.DotSize <= 0 {
		opts.DotSize = 1.0
	}
	if opts.Density <= 0 {
		opts.Density = 1.0
	}
	if opts.Dither == "" {
		opts.Dither = "atkinson"
	}
	if opts.TemporalStability < 0 {
		opts.TemporalStability = 0
	} else if opts.TemporalStability > 1.0 {
		opts.TemporalStability = 1.0
	}

	step := int(math.Round(opts.DotSize))
	if step < 1 {
		step = 1
	}

	total := opts.Width * opts.Height
	gridW := (opts.Width + step - 1) / step
	gridH := (opts.Height + step - 1) / step
	gridTotal := gridW * gridH

	return &Renderer{
		opts:          opts,
		step:          step,
		lumBuf:        make([]float64, total),
		ditherBuf:     make([]float64, total),
		dotsBuf:       make([]bool, total),
		prevLum:       make([]float64, total),
		prevDots:      make([]bool, total),
		gridLum:       make([]float64, gridTotal),
		gridDitherBuf: make([]float64, gridTotal),
		gridDotsBuf:   make([]bool, gridTotal),
		prevGridLum:   make([]float64, gridTotal),
		prevGridDots:  make([]bool, gridTotal),
	}
}

func (r *Renderer) Reset() {
	r.hasPrevFrame = false
	r.hasPrevGrid = false
}

func (r *Renderer) Render(src, dst *image.RGBA) error {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	canvasW := dst.Bounds().Dx()
	canvasH := dst.Bounds().Dy()
	total := canvasW * canvasH

	if len(r.lumBuf) < total {
		r.lumBuf = make([]float64, total)
		r.ditherBuf = make([]float64, total)
		r.dotsBuf = make([]bool, total)
		r.prevLum = make([]float64, total)
		r.prevDots = make([]bool, total)
	}

	r.sampleLuminance(src, srcW, srcH, canvasW, canvasH)

	if r.step <= 1 {
		r.renderSinglePixel(dst, canvasW, canvasH)
	} else {
		r.renderDiscs(dst, canvasW, canvasH)
	}

	return nil
}

func (r *Renderer) sampleLuminance(src *image.RGBA, srcW, srcH, canvasW, canvasH int) {
	srcPix := src.Pix
	srcStride := src.Stride
	lum := r.lumBuf
	total := canvasW * canvasH

	if srcW == canvasW && srcH == canvasH {
		for cy := 0; cy < canvasH; cy++ {
			rowOff := cy * srcStride
			dstOff := cy * canvasW
			for cx := 0; cx < canvasW; cx++ {
				p := rowOff + cx*4
				lum[dstOff+cx] = calculateLuminance(srcPix[p], srcPix[p+1], srcPix[p+2])
			}
		}
	} else if canvasW < srcW && canvasH < srcH {
		// Area box-averaging to prevent aliasing when downsampling
		for cy := 0; cy < canvasH; cy++ {
			sy0 := (cy * srcH) / canvasH
			sy1 := ((cy + 1) * srcH) / canvasH
			if sy1 <= sy0 {
				sy1 = sy0 + 1
			}
			dstRowOff := cy * canvasW

			for cx := 0; cx < canvasW; cx++ {
				sx0 := (cx * srcW) / canvasW
				sx1 := ((cx + 1) * srcW) / canvasW
				if sx1 <= sx0 {
					sx1 = sx0 + 1
				}

				var totalR, totalG, totalB, count int
				for sy := sy0; sy < sy1; sy++ {
					rowOff := sy * srcStride
					for sx := sx0; sx < sx1; sx++ {
						p := rowOff + sx*4
						totalR += int(srcPix[p])
						totalG += int(srcPix[p+1])
						totalB += int(srcPix[p+2])
						count++
					}
				}

				avgR := uint8(totalR / count)
				avgG := uint8(totalG / count)
				avgB := uint8(totalB / count)
				lum[dstRowOff+cx] = calculateLuminance(avgR, avgG, avgB)
			}
		}
	} else {
		for cy := 0; cy < canvasH; cy++ {
			sy := (cy * srcH) / canvasH
			srcRowOff := sy * srcStride
			dstRowOff := cy * canvasW
			for cx := 0; cx < canvasW; cx++ {
				sx := (cx * srcW) / canvasW
				p := srcRowOff + sx*4
				lum[dstRowOff+cx] = calculateLuminance(srcPix[p], srcPix[p+1], srcPix[p+2])
			}
		}
	}

	// Ignore luminance changes within the deadband to suppress sensor noise
	if r.hasPrevFrame && r.opts.TemporalStability > 0 {
		deadband := r.opts.TemporalStability * 0.08
		prevLum := r.prevLum
		for i := 0; i < total; i++ {
			if math.Abs(lum[i]-prevLum[i]) < deadband {
				lum[i] = prevLum[i]
			}
		}
	}
}

// ITU-R BT.709 perceptual luminance
func calculateLuminance(r, g, b uint8) float64 {
	y := (2126*float64(r) + 7152*float64(g) + 722*float64(b)) / (10000.0 * 255.0)

	if y <= ShadowCutoff {
		return 0.0
	}
	return y
}

func (r *Renderer) renderSinglePixel(dst *image.RGBA, w, h int) {
	total := w * h
	var prevDots []bool
	if r.hasPrevFrame {
		prevDots = r.prevDots[:total]
	}

	Dither(r.lumBuf[:total], r.dotsBuf[:total], r.ditherBuf[:total], w, h, r.opts.Dither, r.opts.Density, prevDots, r.opts.TemporalStability)

	pix := dst.Pix
	fg := r.opts.Foreground
	bg := r.opts.Background

	for i := 0; i < total; i++ {
		p := i * 4
		if r.dotsBuf[i] {
			pix[p] = fg.R
			pix[p+1] = fg.G
			pix[p+2] = fg.B
			pix[p+3] = fg.A
		} else {
			pix[p] = bg.R
			pix[p+1] = bg.G
			pix[p+2] = bg.B
			pix[p+3] = bg.A
		}
	}

	copy(r.prevLum[:total], r.lumBuf[:total])
	copy(r.prevDots[:total], r.dotsBuf[:total])
	r.hasPrevFrame = true
}

func (r *Renderer) renderDiscs(dst *image.RGBA, w, h int) {
	step := r.step
	gridW := (w + step - 1) / step
	gridH := (h + step - 1) / step
	gridTotal := gridW * gridH

	if len(r.gridLum) < gridTotal {
		r.gridLum = make([]float64, gridTotal)
		r.gridDitherBuf = make([]float64, gridTotal)
		r.gridDotsBuf = make([]bool, gridTotal)
		r.prevGridLum = make([]float64, gridTotal)
		r.prevGridDots = make([]bool, gridTotal)
	}

	for gy := 0; gy < gridH; gy++ {
		y0 := gy * step
		y1 := y0 + step
		if y1 > h {
			y1 = h
		}
		gridRow := gy * gridW
		for gx := 0; gx < gridW; gx++ {
			x0 := gx * step
			x1 := x0 + step
			if x1 > w {
				x1 = w
			}

			var sum float64
			var count int
			for cy := y0; cy < y1; cy++ {
				rowOff := cy * w
				for cx := x0; cx < x1; cx++ {
					sum += r.lumBuf[rowOff+cx]
					count++
				}
			}
			if count > 0 {
				r.gridLum[gridRow+gx] = sum / float64(count)
			} else {
				r.gridLum[gridRow+gx] = 0
			}
		}
	}

	if r.hasPrevGrid && r.opts.TemporalStability > 0 {
		deadband := r.opts.TemporalStability * 0.08
		for i := 0; i < gridTotal; i++ {
			if math.Abs(r.gridLum[i]-r.prevGridLum[i]) < deadband {
				r.gridLum[i] = r.prevGridLum[i]
			}
		}
	}

	var prevGridDots []bool
	if r.hasPrevGrid {
		prevGridDots = r.prevGridDots[:gridTotal]
	}

	Dither(r.gridLum[:gridTotal], r.gridDotsBuf[:gridTotal], r.gridDitherBuf[:gridTotal], gridW, gridH, r.opts.Dither, r.opts.Density, prevGridDots, r.opts.TemporalStability)

	pix := dst.Pix
	total := w * h
	bg := r.opts.Background
	fg := r.opts.Foreground

	for i := 0; i < total; i++ {
		p := i * 4
		pix[p] = bg.R
		pix[p+1] = bg.G
		pix[p+2] = bg.B
		pix[p+3] = bg.A
	}

	halfStep := float64(step) * 0.5
	rSq := halfStep * halfStep

	for gy := 0; gy < gridH; gy++ {
		y0 := gy * step
		y1 := y0 + step
		if y1 > h {
			y1 = h
		}
		cy := float64(y0) + halfStep
		gridRow := gy * gridW

		for gx := 0; gx < gridW; gx++ {
			if !r.gridDotsBuf[gridRow+gx] {
				continue
			}

			x0 := gx * step
			x1 := x0 + step
			if x1 > w {
				x1 = w
			}
			cx := float64(x0) + halfStep

			for py := y0; py < y1; py++ {
				dy := float64(py) + 0.5 - cy
				dySq := dy * dy
				rowOff := py * w * 4
				for px := x0; px < x1; px++ {
					dx := float64(px) + 0.5 - cx
					if step <= 3 || (dx*dx+dySq) <= rSq*1.05 {
						offset := rowOff + px*4
						pix[offset] = fg.R
						pix[offset+1] = fg.G
						pix[offset+2] = fg.B
						pix[offset+3] = fg.A
					}
				}
			}
		}
	}

	copy(r.prevGridLum[:gridTotal], r.gridLum[:gridTotal])
	copy(r.prevGridDots[:gridTotal], r.gridDotsBuf[:gridTotal])
	r.hasPrevGrid = true

	copy(r.prevLum[:total], r.lumBuf[:total])
	r.hasPrevFrame = true
}
