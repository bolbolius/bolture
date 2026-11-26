package render

import "strings"

// algorithmNames lists the --dither values shown in help output. The first
// entry is the default.
var algorithmNames = []string{
	"atkinson",
	"floyd-steinberg",
	"burkes",
	"sierra",
	"stucki",
	"jarvis",
	"bayer4",
	"bayer8",
	"bayer16",
}

// Algorithms returns the accepted --dither values.
func Algorithms() []string {
	out := make([]string, len(algorithmNames))
	copy(out, algorithmNames)
	return out
}

// ParseAlgorithm maps a --dither value and its aliases to a canonical name.
func ParseAlgorithm(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "atkinson":
		return "atkinson", true
	case "floyd-steinberg", "floydsteinberg", "fs":
		return "floyd-steinberg", true
	case "burkes":
		return "burkes", true
	case "sierra", "sierra3", "sierra-3":
		return "sierra", true
	case "stucki":
		return "stucki", true
	case "jarvis", "jjn", "jarvis-judice-ninke":
		return "jarvis", true
	case "bayer4", "bayer4x4":
		return "bayer4", true
	case "bayer", "bayer8", "bayer8x8":
		return "bayer8", true
	case "bayer16", "bayer16x16":
		return "bayer16", true
	}
	return "", false
}

// Ordered dithering matrices. bayerMatrix builds the recursive threshold
// matrix, normalized to [0, 1), so a pixel turns into a dot when
// luminance * density is above its cell.
var (
	bayer4  = bayerMatrix(4)
	bayer8  = bayerMatrix(8)
	bayer16 = bayerMatrix(16)
)

func bayerMatrix(n int) []float64 {
	m := []float64{0, 2, 3, 1}
	size := 2
	for size < n {
		next := make([]float64, (size*2)*(size*2))
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				v := 4 * m[y*size+x]
				next[y*size*2+x] = v
				next[y*size*2+x+size] = v + 2
				next[(y+size)*size*2+x] = v + 3
				next[(y+size)*size*2+x+size] = v + 1
			}
		}
		m = next
		size *= 2
	}
	denom := float64(n * n)
	for i := range m {
		m[i] /= denom
	}
	return m
}

// tap is one error-diffusion neighbor. weight is relative to the kernel
// divisor until newDiffusionKernel normalizes it.
type tap struct {
	dx, dy int
	weight float64
}

type diffusionKernel struct {
	taps  []tap
	maxDX int
	maxDY int
}

func newDiffusionKernel(divisor int, raw ...tap) *diffusionKernel {
	if len(raw) > 12 {
		panic("diffusion kernel has more taps than the fixed scratch array holds")
	}
	k := &diffusionKernel{}
	scale := 1 / float64(divisor)
	for _, t := range raw {
		t.weight *= scale
		k.taps = append(k.taps, t)
		if t.dx < 0 {
			t.dx = -t.dx
		}
		if t.dx > k.maxDX {
			k.maxDX = t.dx
		}
		if t.dy > k.maxDY {
			k.maxDY = t.dy
		}
	}
	return k
}

// Atkinson throws away a quarter of the error, which gives its lighter,
// sparser look.
var diffusionKernels = map[string]*diffusionKernel{
	"atkinson": newDiffusionKernel(8,
		tap{1, 0, 1}, tap{2, 0, 1},
		tap{-1, 1, 1}, tap{0, 1, 1}, tap{1, 1, 1},
		tap{0, 2, 1},
	),
	"floyd-steinberg": newDiffusionKernel(16,
		tap{1, 0, 7},
		tap{-1, 1, 3}, tap{0, 1, 5}, tap{1, 1, 1},
	),
	"burkes": newDiffusionKernel(32,
		tap{1, 0, 8}, tap{2, 0, 4},
		tap{-2, 1, 2}, tap{-1, 1, 4}, tap{0, 1, 8}, tap{1, 1, 4}, tap{2, 1, 2},
	),
	"sierra": newDiffusionKernel(32,
		tap{1, 0, 5}, tap{2, 0, 3},
		tap{-2, 1, 2}, tap{-1, 1, 4}, tap{0, 1, 5}, tap{1, 1, 4}, tap{2, 1, 2},
		tap{-1, 2, 2}, tap{0, 2, 3}, tap{1, 2, 2},
	),
	"stucki": newDiffusionKernel(42,
		tap{1, 0, 8}, tap{2, 0, 4},
		tap{-2, 1, 2}, tap{-1, 1, 4}, tap{0, 1, 8}, tap{1, 1, 4}, tap{2, 1, 2},
		tap{-2, 2, 1}, tap{-1, 2, 2}, tap{0, 2, 4}, tap{1, 2, 2}, tap{2, 2, 1},
	),
	"jarvis": newDiffusionKernel(48,
		tap{1, 0, 7}, tap{2, 0, 5},
		tap{-2, 1, 3}, tap{-1, 1, 5}, tap{0, 1, 7}, tap{1, 1, 5}, tap{2, 1, 3},
		tap{-2, 2, 1}, tap{-1, 2, 3}, tap{0, 2, 5}, tap{1, 2, 3}, tap{2, 2, 1},
	),
}
