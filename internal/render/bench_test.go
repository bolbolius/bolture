package render

import (
	"image"
	"image/color"
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

func benchRender(b *testing.B, w, h int, alg string) {
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
}

func BenchmarkRender720p(b *testing.B) {
	for _, alg := range Algorithms() {
		b.Run(alg, func(b *testing.B) { benchRender(b, 1280, 720, alg) })
	}
}

func BenchmarkRender1080p(b *testing.B) {
	for _, alg := range Algorithms() {
		b.Run(alg, func(b *testing.B) { benchRender(b, 1920, 1080, alg) })
	}
}
