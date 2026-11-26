package render

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"testing"
)

func benchmarkSource() *image.RGBA {
	src := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			p := y*src.Stride + x*4
			src.Pix[p] = uint8(x)
			src.Pix[p+1] = uint8(y)
			src.Pix[p+2] = uint8((x + y) / 2)
			src.Pix[p+3] = 255
		}
	}
	return src
}

func benchRender(b *testing.B, w, h int, alg string, threads int) {
	b.Helper()
	src := benchmarkSource()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	r := NewRenderer(Options{
		Width:             w,
		Height:            h,
		DotSize:           1.0,
		Density:           1.0,
		Dither:            alg,
		TemporalStability: 0.6,
		Foreground:        color.RGBA{240, 216, 192, 255},
		Background:        color.RGBA{0, 0, 0, 0},
		Threads:           threads,
	})

	// Warm the sampling maps so the reported figure is the steady-state
	// playback path rather than the first frame at a new size.
	if err := r.Render(src, dst); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := r.Render(src, dst); err != nil {
			b.Fatal(err)
		}
	}
	r.Close()
}

func BenchmarkRender720p(b *testing.B) {
	for _, alg := range Algorithms() {
		b.Run(alg, func(b *testing.B) { benchRender(b, 1280, 720, alg, 1) })
	}
}

func BenchmarkRender1080p(b *testing.B) {
	for _, alg := range Algorithms() {
		b.Run(alg, func(b *testing.B) { benchRender(b, 1920, 1080, alg, 1) })
	}
}

func BenchmarkRender720pParallel(b *testing.B) {
	if runtime.NumCPU() < 2 {
		b.Skip("single CPU")
	}
	threads := []int{2, 4, runtime.NumCPU()}
	for _, alg := range []string{"atkinson", "bayer8", "jarvis"} {
		for _, n := range threads {
			b.Run(fmt.Sprintf("%s/%d", alg, n), func(b *testing.B) {
				benchRender(b, 1280, 720, alg, n)
			})
		}
	}
}

// BenchmarkFrame measures the full frame path: sampling, dithering, and the
// pixel write, from one 1080p source at each canvas size.
func BenchmarkFrame(b *testing.B) {
	src := benchmarkSource()
	canvases := []struct {
		name string
		w, h int
	}{
		{"360p", 640, 360},
		{"720p", 1280, 720},
		{"1080p", 1920, 1080},
	}
	for _, c := range canvases {
		for _, threads := range []int{1, 4} {
			b.Run(fmt.Sprintf("%s/%d", c.name, threads), func(b *testing.B) {
				dst := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
				r := NewRenderer(Options{
					Width:             c.w,
					Height:            c.h,
					DotSize:           1,
					Density:           1,
					Dither:            "atkinson",
					TemporalStability: 0.6,
					Foreground:        color.RGBA{240, 216, 192, 255},
					Background:        color.RGBA{0, 0, 0, 0},
					Threads:           threads,
				})
				if err := r.Render(src, dst); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := r.Render(src, dst); err != nil {
						b.Fatal(err)
					}
				}
				r.Close()
			})
		}
	}
}

// BenchmarkDraw measures the dither and pixel write for a prepared luminance
// buffer, with sampling excluded.
func BenchmarkDraw(b *testing.B) {
	canvases := []struct {
		name string
		w, h int
	}{
		{"360p", 640, 360},
		{"720p", 1280, 720},
		{"1080p", 1920, 1080},
	}
	for _, c := range canvases {
		b.Run(c.name, func(b *testing.B) {
			lum := make([]float64, c.w*c.h)
			for i := range lum {
				lum[i] = float64((i*37)%1000) / 1000
			}
			dst := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
			r := NewRenderer(Options{
				Width: c.w, Height: c.h, DotSize: 1, Density: 1,
				Dither: "atkinson", TemporalStability: 0.6,
				Foreground: color.RGBA{240, 216, 192, 255},
				Background: color.RGBA{0, 0, 0, 0},
			})
			copy(r.lumBuf, lum)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r.renderSinglePixel(dst, c.w, c.h)
			}
		})
	}
}

// BenchmarkDitherOnly measures the diffusion scan alone, with luminance and
// the dot buffer prepared.
func BenchmarkDitherOnly(b *testing.B) {
	canvases := []struct {
		name string
		w, h int
	}{
		{"360p", 640, 360},
		{"720p", 1280, 720},
		{"1080p", 1920, 1080},
	}
	for _, c := range canvases {
		b.Run(c.name, func(b *testing.B) {
			lum := make([]float64, c.w*c.h)
			for i := range lum {
				lum[i] = float64((i*37)%1000) / 1000
			}
			dots := make([]bool, c.w*c.h)
			work := make([]float64, c.w*c.h)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				Dither(lum, dots, work, c.w, c.h, "atkinson", 1, nil, 0)
			}
		})
	}
}
