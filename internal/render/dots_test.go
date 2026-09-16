package render

import (
	"image"
	"image/color"
	"testing"
)

func TestRenderer(t *testing.T) {
	opts := Options{
		Width:      100,
		Height:     100,
		DotSize:    1.0,
		Density:    1.0,
		Dither:     "atkinson",
		Foreground: color.RGBA{240, 216, 192, 255},
		Background: color.RGBA{0, 0, 0, 0},
	}
	r := NewRenderer(opts)

	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if y >= 50 {
				src.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			} else {
				src.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, 100, 100))
	if err := r.Render(src, dst); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	for y := 0; y < 45; y++ {
		for x := 0; x < 100; x++ {
			c := dst.RGBAAt(x, y)
			if c.A != 0 {
				t.Fatalf("expected pure black to be transparent at (%d, %d), got %v", x, y, c)
			}
		}
	}

	bottomDots := 0
	for y := 55; y < 100; y++ {
		for x := 0; x < 100; x++ {
			c := dst.RGBAAt(x, y)
			if c == opts.Foreground {
				bottomDots++
			}
		}
	}
	if bottomDots == 0 {
		t.Fatalf("expected foreground dots in bright bottom half, got 0")
	}
}

func TestRendererDownsampling(t *testing.T) {
	opts := Options{
		Width:      640,
		Height:     360,
		DotSize:    1.0,
		Density:    1.0,
		Dither:     "atkinson",
		Foreground: color.RGBA{255, 255, 255, 255},
		Background: color.RGBA{0, 0, 0, 0},
	}
	r := NewRenderer(opts)

	src := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			if x >= 640 {
				src.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			} else {
				src.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, 640, 360))
	if err := r.Render(src, dst); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	for y := 0; y < 360; y++ {
		for x := 0; x < 300; x++ {
			if dst.RGBAAt(x, y).A != 0 {
				t.Fatalf("expected left side to be transparent at (%d, %d), got %v", x, y, dst.RGBAAt(x, y))
			}
		}
	}

	rightDots := 0
	for y := 0; y < 360; y++ {
		for x := 340; x < 640; x++ {
			if dst.RGBAAt(x, y) == opts.Foreground {
				rightDots++
			}
		}
	}
	if rightDots == 0 {
		t.Fatalf("expected dots on right side, got 0")
	}
}

func TestTemporalStabilityAtkinson(t *testing.T) {
	rOff := NewRenderer(Options{
		Width:             80,
		Height:            80,
		DotSize:           1.0,
		Density:           1.0,
		Dither:            "atkinson",
		TemporalStability: 0.0,
		Foreground:        color.RGBA{255, 255, 255, 255},
		Background:        color.RGBA{0, 0, 0, 0},
	})

	src1 := image.NewRGBA(image.Rect(0, 0, 80, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			v := uint8(100 + (x%10)*5)
			src1.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}

	src2 := image.NewRGBA(image.Rect(0, 0, 80, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			v := uint8(100 + (x%10)*5)
			noise := int8(((x*37 + y*73) % 7) - 3)
			vNoisy := uint8(int(v) + int(noise))
			src2.SetRGBA(x, y, color.RGBA{vNoisy, vNoisy, vNoisy, 255})
		}
	}

	dst1Off := image.NewRGBA(image.Rect(0, 0, 80, 80))
	dst2Off := image.NewRGBA(image.Rect(0, 0, 80, 80))
	_ = rOff.Render(src1, dst1Off)
	_ = rOff.Render(src2, dst2Off)

	flipsOff := 0
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			if dst1Off.RGBAAt(x, y) != dst2Off.RGBAAt(x, y) {
				flipsOff++
			}
		}
	}
	if flipsOff == 0 {
		t.Fatalf("expected unstable Atkinson to have flipped dots under sensor noise, got 0")
	}

	rStable := NewRenderer(Options{
		Width:             80,
		Height:            80,
		DotSize:           1.0,
		Density:           1.0,
		Dither:            "atkinson",
		TemporalStability: 0.6,
		Foreground:        color.RGBA{255, 255, 255, 255},
		Background:        color.RGBA{0, 0, 0, 0},
	})

	dst1Stable := image.NewRGBA(image.Rect(0, 0, 80, 80))
	dst2Stable := image.NewRGBA(image.Rect(0, 0, 80, 80))
	_ = rStable.Render(src1, dst1Stable)
	_ = rStable.Render(src2, dst2Stable)

	flipsStable := 0
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			if dst1Stable.RGBAAt(x, y) != dst2Stable.RGBAAt(x, y) {
				flipsStable++
			}
		}
	}
	if flipsStable != 0 {
		t.Fatalf("expected 0 dot flips on static noisy scene with temporal stability, got %d", flipsStable)
	}
}

func TestTemporalStabilityMotionResponse(t *testing.T) {
	r := NewRenderer(Options{
		Width:             60,
		Height:            60,
		DotSize:           1.0,
		Density:           1.0,
		Dither:            "atkinson",
		TemporalStability: 0.6,
		Foreground:        color.RGBA{255, 255, 255, 255},
		Background:        color.RGBA{0, 0, 0, 0},
	})

	src1 := image.NewRGBA(image.Rect(0, 0, 60, 60))
	dst1 := image.NewRGBA(image.Rect(0, 0, 60, 60))
	_ = r.Render(src1, dst1)

	src2 := image.NewRGBA(image.Rect(0, 0, 60, 60))
	for y := 20; y < 40; y++ {
		for x := 20; x < 40; x++ {
			src2.SetRGBA(x, y, color.RGBA{240, 240, 240, 255})
		}
	}
	dst2 := image.NewRGBA(image.Rect(0, 0, 60, 60))
	_ = r.Render(src2, dst2)

	spotlightDots := 0
	for y := 20; y < 40; y++ {
		for x := 20; x < 40; x++ {
			if dst2.RGBAAt(x, y).A > 0 {
				spotlightDots++
			}
		}
	}
	if spotlightDots == 0 {
		t.Fatalf("expected dots to update immediately on motion, got 0")
	}
}

func BenchmarkRenderer360p(b *testing.B) {
	opts := Options{
		Width:      640,
		Height:     360,
		DotSize:    1.0,
		Density:    1.0,
		Dither:     "atkinson",
		Foreground: color.RGBA{240, 216, 192, 255},
		Background: color.RGBA{0, 0, 0, 0},
	}
	r := NewRenderer(opts)
	src := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	dst := image.NewRGBA(image.Rect(0, 0, 640, 360))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := r.Render(src, dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderer720p(b *testing.B) {
	opts := Options{
		Width:      1280,
		Height:     720,
		DotSize:    1.0,
		Density:    1.0,
		Dither:     "atkinson",
		Foreground: color.RGBA{240, 216, 192, 255},
		Background: color.RGBA{0, 0, 0, 0},
	}
	r := NewRenderer(opts)
	src := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	dst := image.NewRGBA(image.Rect(0, 0, 1280, 720))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := r.Render(src, dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderer1080p(b *testing.B) {
	opts := Options{
		Width:      1920,
		Height:     1080,
		DotSize:    1.0,
		Density:    1.0,
		Dither:     "atkinson",
		Foreground: color.RGBA{240, 216, 192, 255},
		Background: color.RGBA{0, 0, 0, 0},
	}
	r := NewRenderer(opts)
	src := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := r.Render(src, dst); err != nil {
			b.Fatal(err)
		}
	}
}
