package render

var bayer8x8 = [8][8]float64{
	{0.0 / 64, 32.0 / 64, 8.0 / 64, 40.0 / 64, 2.0 / 64, 34.0 / 64, 10.0 / 64, 42.0 / 64},
	{48.0 / 64, 16.0 / 64, 56.0 / 64, 24.0 / 64, 50.0 / 64, 18.0 / 64, 58.0 / 64, 26.0 / 64},
	{12.0 / 64, 44.0 / 64, 4.0 / 64, 36.0 / 64, 14.0 / 64, 46.0 / 64, 6.0 / 64, 38.0 / 64},
	{60.0 / 64, 28.0 / 64, 52.0 / 64, 20.0 / 64, 62.0 / 64, 30.0 / 64, 54.0 / 64, 22.0 / 64},
	{3.0 / 64, 35.0 / 64, 11.0 / 64, 43.0 / 64, 1.0 / 64, 33.0 / 64, 9.0 / 64, 41.0 / 64},
	{51.0 / 64, 19.0 / 64, 59.0 / 64, 27.0 / 64, 49.0 / 64, 17.0 / 64, 57.0 / 64, 25.0 / 64},
	{15.0 / 64, 47.0 / 64, 7.0 / 64, 39.0 / 64, 13.0 / 64, 45.0 / 64, 5.0 / 64, 37.0 / 64},
	{63.0 / 64, 31.0 / 64, 55.0 / 64, 23.0 / 64, 61.0 / 64, 29.0 / 64, 53.0 / 64, 21.0 / 64},
}

func Dither(lum []float64, dots []bool, workBuf []float64, w, h int, algorithm string, density float64, prevDots []bool, stability float64) {
	if algorithm == "bayer" {
		ditherBayer(lum, dots, w, h, density, prevDots, stability)
		return
	}
	ditherAtkinson(lum, dots, workBuf, w, h, density, prevDots, stability)
}

func ditherBayer(lum []float64, dots []bool, w, h int, density float64, prevDots []bool, stability float64) {
	hysteresis := stability * 0.05
	hasPrev := len(prevDots) == w*h && stability > 0

	for y := 0; y < h; y++ {
		rowIdx := y * w
		by := y % 8
		for x := 0; x < w; x++ {
			idx := rowIdx + x
			l := lum[idx]
			if l <= 0.0 {
				dots[idx] = false
				continue
			}

			bx := x % 8
			threshold := bayer8x8[by][bx]
			if hasPrev {
				if prevDots[idx] {
					threshold -= hysteresis
				} else {
					threshold += hysteresis
				}
			}
			dots[idx] = (l * density) > threshold
		}
	}
}

func ditherAtkinson(lum []float64, dots []bool, workBuf []float64, w, h int, density float64, prevDots []bool, stability float64) {
	total := w * h
	var buf []float64
	if len(workBuf) >= total {
		buf = workBuf[:total]
	} else {
		buf = make([]float64, total)
	}

	for i := 0; i < total; i++ {
		buf[i] = lum[i] * density
	}

	stickiness := stability * 0.25
	hasPrev := len(prevDots) == total && stability > 0

	for y := 0; y < h; y++ {
		rowIdx := y * w
		if y%2 == 0 {
			for x := 0; x < w; x++ {
				idx := rowIdx + x
				val := buf[idx]

				threshold := 0.5
				if hasPrev {
					if prevDots[idx] {
						threshold -= stickiness
					} else {
						threshold += stickiness
					}
				}

				var newVal float64
				if val >= threshold {
					newVal = 1.0
					dots[idx] = true
				} else {
					newVal = 0.0
					dots[idx] = false
				}

				err := (val - newVal) / 8.0

				if x+1 < w {
					buf[idx+1] += err
				}
				if x+2 < w {
					buf[idx+2] += err
				}
				if y+1 < h {
					if x-1 >= 0 {
						buf[idx+w-1] += err
					}
					buf[idx+w] += err
					if x+1 < w {
						buf[idx+w+1] += err
					}
				}
				if y+2 < h {
					buf[idx+2*w] += err
				}
			}
		} else {
			// Serpentine scan right-to-left to reduce directional bias
			for x := w - 1; x >= 0; x-- {
				idx := rowIdx + x
				val := buf[idx]

				threshold := 0.5
				if hasPrev {
					if prevDots[idx] {
						threshold -= stickiness
					} else {
						threshold += stickiness
					}
				}

				var newVal float64
				if val >= threshold {
					newVal = 1.0
					dots[idx] = true
				} else {
					newVal = 0.0
					dots[idx] = false
				}

				err := (val - newVal) / 8.0

				if x-1 >= 0 {
					buf[idx-1] += err
				}
				if x-2 >= 0 {
					buf[idx-2] += err
				}
				if y+1 < h {
					if x+1 < w {
						buf[idx+w+1] += err
					}
					buf[idx+w] += err
					if x-1 >= 0 {
						buf[idx+w-1] += err
					}
				}
				if y+2 < h {
					buf[idx+2*w] += err
				}
			}
		}
	}
}
