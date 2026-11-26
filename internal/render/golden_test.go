package render

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"testing"
)

func goldenSource() *image.RGBA {
	src := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 160; x++ {
			p := y*src.Stride + x*4
			src.Pix[p] = uint8(x * 3)
			src.Pix[p+1] = uint8(y * 5)
			src.Pix[p+2] = uint8((x * y) / 7)
			src.Pix[p+3] = 255
		}
	}
	return src
}

// These hashes pin the output of the original algorithms. The refactor that
// added more kernels must not change what --dither atkinson or --dither bayer
// already produced.
func TestGoldenOutput(t *testing.T) {
	cases := []struct {
		algorithm string
		dotSize   float64
		want      uint64
	}{
		{"atkinson", 1, 0x673241c8603d4085},
		{"atkinson", 2, 0x2b9b1c18c6814be5},
		{"bayer", 1, 0xe775dfdf5b3de300},
		{"bayer", 2, 0xb8a217613127a925},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s-dot%g", tc.algorithm, tc.dotSize), func(t *testing.T) {
			r := NewRenderer(Options{
				Width:      128,
				Height:     96,
				DotSize:    tc.dotSize,
				Density:    1,
				Dither:     tc.algorithm,
				Foreground: color.RGBA{240, 216, 192, 255},
				Background: color.RGBA{0, 0, 0, 0},
			})
			dst := image.NewRGBA(image.Rect(0, 0, 128, 96))
			if err := r.Render(goldenSource(), dst); err != nil {
				t.Fatal(err)
			}
			h := fnv.New64a()
			h.Write(dst.Pix)
			if got := h.Sum64(); got != tc.want {
				t.Errorf("output hash = %x, want %x", got, tc.want)
			}
		})
	}
}
