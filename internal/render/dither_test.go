package render

import (
	"hash/fnv"
	"image"
	"image/color"
	"testing"
)

func testGradient(w, h int) *image.RGBA {
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*src.Stride + x*4
			src.Pix[p] = uint8(x * 255 / (w - 1))
			src.Pix[p+1] = uint8(y * 255 / (h - 1))
			src.Pix[p+2] = uint8((x + y) * 255 / (w + h - 2))
			src.Pix[p+3] = 255
		}
	}
	return src
}

func TestParseAlgorithm(t *testing.T) {
	cases := map[string]string{
		"":                "atkinson",
		"ATKINSON":        "atkinson",
		"fs":              "floyd-steinberg",
		"Floyd-Steinberg": "floyd-steinberg",
		"burkes":          "burkes",
		"sierra3":         "sierra",
		"stucki":          "stucki",
		"jjn":             "jarvis",
		"bayer":           "bayer8",
		"bayer8x8":        "bayer8",
		"bayer4":          "bayer4",
		"bayer16":         "bayer16",
	}
	for input, want := range cases {
		got, ok := ParseAlgorithm(input)
		if !ok || got != want {
			t.Errorf("ParseAlgorithm(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
	if _, ok := ParseAlgorithm("garbage"); ok {
		t.Error("ParseAlgorithm accepted an unknown name")
	}
}

func TestBayerMatrices(t *testing.T) {
	matrices := []struct {
		n int
		m []float64
	}{
		{4, bayer4},
		{8, bayer8},
		{16, bayer16},
	}
	for _, tc := range matrices {
		if len(tc.m) != tc.n*tc.n {
			t.Fatalf("bayer%d has %d cells, want %d", tc.n, len(tc.m), tc.n*tc.n)
		}
		seen := make(map[float64]bool, len(tc.m))
		for _, v := range tc.m {
			if v < 0 || v >= 1 {
				t.Fatalf("bayer%d value %v outside [0, 1)", tc.n, v)
			}
			if seen[v] {
				t.Fatalf("bayer%d repeats %v", tc.n, v)
			}
			seen[v] = true
		}
	}

	// The first row of the original hardcoded 8x8 matrix.
	want := []float64{0, 32, 8, 40, 2, 34, 10, 42}
	for i, v := range want {
		if bayer8[i] != v/64 {
			t.Errorf("bayer8[%d] = %v, want %v", i, bayer8[i], v/64)
		}
	}
}

func TestEveryAlgorithmRenders(t *testing.T) {
	opts := Options{
		Width:             160,
		Height:            96,
		Density:           1,
		TemporalStability: 0,
		Foreground:        color.RGBA{240, 216, 192, 255},
		Background:        color.RGBA{0, 0, 0, 0},
	}
	src := testGradient(160, 96)

	for _, name := range Algorithms() {
		t.Run(name, func(t *testing.T) {
			for _, dotSize := range []float64{1, 2} {
				opts.Dither = name
				opts.DotSize = dotSize
				r := NewRenderer(opts)
				dst := image.NewRGBA(image.Rect(0, 0, 160, 96))
				if err := r.Render(src, dst); err != nil {
					t.Fatal(err)
				}

				first := fnv.New64a()
				first.Write(dst.Pix)

				if err := r.Render(src, dst); err != nil {
					t.Fatal(err)
				}
				second := fnv.New64a()
				second.Write(dst.Pix)
				if first.Sum64() != second.Sum64() {
					t.Fatalf("dot size %g is not deterministic", dotSize)
				}

				dots := 0
				for y := 0; y < 96; y++ {
					for x := 0; x < 160; x++ {
						c := dst.RGBAAt(x, y)
						switch c {
						case opts.Foreground:
							dots++
						case opts.Background:
						default:
							t.Fatalf("dot size %g: unexpected pixel %v at (%d, %d)", dotSize, c, x, y)
						}
					}
				}
				if dots == 0 || dots == 160*96 {
					t.Fatalf("dot size %g: %d dots, want a mix", dotSize, dots)
				}
			}
		})
	}
}

func TestRenderDoesNotAllocate(t *testing.T) {
	src := testGradient(320, 180)
	dst := image.NewRGBA(image.Rect(0, 0, 160, 90))
	r := NewRenderer(Options{
		Width:             160,
		Height:            90,
		Density:           1,
		Dither:            "jarvis",
		TemporalStability: 0.6,
		Foreground:        color.RGBA{240, 216, 192, 255},
		Background:        color.RGBA{0, 0, 0, 0},
	})

	allocs := testing.AllocsPerRun(10, func() {
		if err := r.Render(src, dst); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("Render allocated %v times per run, want 0", allocs)
	}
}
