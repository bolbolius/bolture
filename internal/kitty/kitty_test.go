package kitty

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestPresenter(t *testing.T) {
	p := NewPresenter()
	defer p.Cleanup()

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.SetRGBA(x, y, color.RGBA{240, 216, 192, 255})
		}
	}

	var buf bytes.Buffer
	place := Placement{
		WidthPx:  20,
		HeightPx: 20,
		StartCol: 5,
		StartRow: 3,
	}
	if err := p.SendFrame(&buf, img, 1, place); err != nil {
		t.Fatalf("SendFrame failed: %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "\x1b[3;5H\x1b_Ga=T,f=32,s=20,v=20") {
		t.Errorf("unexpected header: %q", output[:40])
	}
	if strings.Contains(output, "c=") || strings.Contains(output, "r=") {
		t.Errorf("expected 1:1 mapping (no c/r), got %q", output)
	}

	var delBuf bytes.Buffer
	p.DeleteImage(&delBuf, 1)
	if delBuf.String() != "\x1b_Ga=d,d=i,i=1\x1b\\" {
		t.Errorf("unexpected delete sequence: %q", delBuf.String())
	}
}

func TestScaleNearest(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	fg := color.RGBA{240, 216, 192, 255}
	bg := color.RGBA{0, 0, 0, 0}
	src.SetRGBA(0, 0, fg)
	src.SetRGBA(1, 0, bg)
	src.SetRGBA(0, 1, bg)
	src.SetRGBA(1, 1, fg)

	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	ScaleNearest(src, dst)

	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if dst.RGBAAt(x, y) != fg {
				t.Errorf("expected fg at (%d, %d), got %v", x, y, dst.RGBAAt(x, y))
			}
		}
	}
	for y := 0; y < 2; y++ {
		for x := 2; x < 4; x++ {
			if dst.RGBAAt(x, y) != bg {
				t.Errorf("expected bg at (%d, %d), got %v", x, y, dst.RGBAAt(x, y))
			}
		}
	}
}

func BenchmarkScaleNearest1080p(b *testing.B) {
	src := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ScaleNearest(src, dst)
	}
}
