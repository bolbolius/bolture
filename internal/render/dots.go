package render

import (
	"encoding/binary"
	"image"
	"image/color"
	"math"
	"runtime"
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
	// Threads is the number of workers for sampling and drawing. 1 keeps the
	// renderer single-core. 0 and negative values are treated as 1.
	Threads int
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

	// Source-to-canvas sampling maps, rebuilt only when a dimension changes.
	sampleReady            bool
	sampleMode             int
	sampleSrcW, sampleSrcH int
	sampleDstW, sampleDstH int
	sampleX0, sampleX1     []int
	sampleY0, sampleY1     []int
	sampleCol, sampleRow   []int

	pool      *parallelPool
	sampleSrc *image.RGBA
	frameW    int
	framePix  []byte
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
	if name, ok := ParseAlgorithm(opts.Dither); ok {
		opts.Dither = name
	} else {
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

	r := &Renderer{
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

		sampleX0:  make([]int, opts.Width),
		sampleX1:  make([]int, opts.Width),
		sampleCol: make([]int, opts.Width),
		sampleY0:  make([]int, opts.Height),
		sampleY1:  make([]int, opts.Height),
		sampleRow: make([]int, opts.Height),
	}

	threads := opts.Threads
	if threads < 2 {
		return r
	}
	if max := runtime.NumCPU(); threads > max {
		threads = max
	}
	if threads > 1 {
		r.pool = newParallelPool(r, threads)
	}
	return r
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

const (
	sampleIdentity = iota
	sampleBox
	sampleNearest
)

// prepareSampling builds the source-to-canvas index maps for the current
// dimensions. The slices are owned by the renderer and reused, so this stays
// off the allocation path after the first frame at a given size.
func (r *Renderer) prepareSampling(srcW, srcH, dstW, dstH int) {
	if r.sampleReady && r.sampleSrcW == srcW && r.sampleSrcH == srcH &&
		r.sampleDstW == dstW && r.sampleDstH == dstH {
		return
	}

	if cap(r.sampleX0) < dstW {
		r.sampleX0 = make([]int, dstW)
		r.sampleX1 = make([]int, dstW)
		r.sampleCol = make([]int, dstW)
	} else {
		r.sampleX0 = r.sampleX0[:dstW]
		r.sampleX1 = r.sampleX1[:dstW]
		r.sampleCol = r.sampleCol[:dstW]
	}
	if cap(r.sampleY0) < dstH {
		r.sampleY0 = make([]int, dstH)
		r.sampleY1 = make([]int, dstH)
		r.sampleRow = make([]int, dstH)
	} else {
		r.sampleY0 = r.sampleY0[:dstH]
		r.sampleY1 = r.sampleY1[:dstH]
		r.sampleRow = r.sampleRow[:dstH]
	}

	switch {
	case srcW == dstW && srcH == dstH:
		r.sampleMode = sampleIdentity
	case dstW < srcW && dstH < srcH:
		r.sampleMode = sampleBox
		for cx := 0; cx < dstW; cx++ {
			sx0 := cx * srcW / dstW
			sx1 := (cx + 1) * srcW / dstW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			r.sampleX0[cx] = sx0
			r.sampleX1[cx] = sx1
		}
		for cy := 0; cy < dstH; cy++ {
			sy0 := cy * srcH / dstH
			sy1 := (cy + 1) * srcH / dstH
			if sy1 <= sy0 {
				sy1 = sy0 + 1
			}
			r.sampleY0[cy] = sy0
			r.sampleY1[cy] = sy1
		}
	default:
		r.sampleMode = sampleNearest
		for cx := 0; cx < dstW; cx++ {
			r.sampleCol[cx] = cx * srcW / dstW
		}
		for cy := 0; cy < dstH; cy++ {
			r.sampleRow[cy] = cy * srcH / dstH
		}
	}

	r.sampleSrcW = srcW
	r.sampleSrcH = srcH
	r.sampleDstW = dstW
	r.sampleDstH = dstH
	r.sampleReady = true
}

func (r *Renderer) sampleLuminance(src *image.RGBA, srcW, srcH, canvasW, canvasH int) {
	r.prepareSampling(srcW, srcH, canvasW, canvasH)
	r.frameW = canvasW
	r.sampleSrc = src

	if r.pool != nil {
		r.pool.run(stepSample, canvasH)
		return
	}
	r.sampleRows(0, canvasH)
}

func (r *Renderer) sampleRows(lo, hi int) {
	srcPix := r.sampleSrc.Pix
	srcStride := r.sampleSrc.Stride
	lum := r.lumBuf
	canvasW := r.frameW

	switch r.sampleMode {
	case sampleIdentity:
		for cy := lo; cy < hi; cy++ {
			rowOff := cy * srcStride
			dstOff := cy * canvasW
			for cx := 0; cx < canvasW; cx++ {
				p := rowOff + cx*4
				lum[dstOff+cx] = calculateLuminance(srcPix[p], srcPix[p+1], srcPix[p+2])
			}
		}
	case sampleBox:
		// Area box-averaging to prevent aliasing when downsampling
		for cy := lo; cy < hi; cy++ {
			sy0 := r.sampleY0[cy]
			sy1 := r.sampleY1[cy]
			dstRowOff := cy * canvasW

			for cx := 0; cx < canvasW; cx++ {
				sx0 := r.sampleX0[cx]
				sx1 := r.sampleX1[cx]

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
	default:
		for cy := lo; cy < hi; cy++ {
			sy := r.sampleRow[cy]
			srcRowOff := sy * srcStride
			dstRowOff := cy * canvasW
			for cx := 0; cx < canvasW; cx++ {
				sx := r.sampleCol[cx]
				p := srcRowOff + sx*4
				lum[dstRowOff+cx] = calculateLuminance(srcPix[p], srcPix[p+1], srcPix[p+2])
			}
		}
	}

	// Ignore luminance changes within the deadband to suppress sensor noise
	if r.hasPrevFrame && r.opts.TemporalStability > 0 {
		deadband := r.opts.TemporalStability * 0.08
		prevLum := r.prevLum
		for cy := lo; cy < hi; cy++ {
			start := cy * canvasW
			for i := start; i < start+canvasW; i++ {
				if math.Abs(lum[i]-prevLum[i]) < deadband {
					lum[i] = prevLum[i]
				}
			}
		}
	}
}

// ITU-R BT.709 perceptual luminance
const (
	luminanceDenom     = 10000.0 * 255.0
	luminanceScale     = 1.0 / luminanceDenom
	shadowCutoffWeight = int(ShadowCutoff * luminanceDenom)
)

// ITU-R BT.709 perceptual luminance. The weighted sum is an exact integer, so
// the hot loop pays one conversion and one multiply instead of a divide.
func calculateLuminance(r, g, b uint8) float64 {
	y := 2126*int(r) + 7152*int(g) + 722*int(b)
	if y <= shadowCutoffWeight {
		return 0.0
	}
	return float64(y) * luminanceScale
}

func (r *Renderer) renderSinglePixel(dst *image.RGBA, w, h int) {
	total := w * h
	var prevDots []bool
	if r.hasPrevFrame {
		prevDots = r.prevDots[:total]
	}

	Dither(r.lumBuf[:total], r.dotsBuf[:total], r.ditherBuf[:total], w, h, r.opts.Dither, r.opts.Density, prevDots, r.opts.TemporalStability)

	r.frameW = w
	r.framePix = dst.Pix
	if r.pool != nil {
		r.pool.run(stepFill, h)
	} else {
		r.fillRows(0, h)
	}

	copy(r.prevLum[:total], r.lumBuf[:total])
	copy(r.prevDots[:total], r.dotsBuf[:total])
	r.hasPrevFrame = true
}

func (r *Renderer) fillRows(lo, hi int) {
	pix := r.framePix
	w := r.frameW
	fg := r.opts.Foreground
	bg := r.opts.Background
	fgPix := uint32(fg.R) | uint32(fg.G)<<8 | uint32(fg.B)<<16 | uint32(fg.A)<<24
	bgPix := uint32(bg.R) | uint32(bg.G)<<8 | uint32(bg.B)<<16 | uint32(bg.A)<<24

	for y := lo; y < hi; y++ {
		row := y * w
		for x := 0; x < w; x++ {
			i := row + x
			v := bgPix
			if r.dotsBuf[i] {
				v = fgPix
			}
			binary.LittleEndian.PutUint32(pix[i*4:], v)
		}
	}
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
	fgPix := uint32(fg.R) | uint32(fg.G)<<8 | uint32(fg.B)<<16 | uint32(fg.A)<<24
	bgPix := uint32(bg.R) | uint32(bg.G)<<8 | uint32(bg.B)<<16 | uint32(bg.A)<<24

	for i := 0; i < total; i++ {
		binary.LittleEndian.PutUint32(pix[i*4:], bgPix)
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
						binary.LittleEndian.PutUint32(pix[offset:], fgPix)
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
