package kitty

import (
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var tempFileCounter uint64

type TerminalSize struct {
	Cols     int
	Rows     int
	WidthPx  int
	HeightPx int
}

type Placement struct {
	WidthPx  int
	HeightPx int
	StartCol int
	StartRow int
}

func GetWindowSize() TerminalSize {
	type winsize struct {
		Row, Col, Xpixel, Ypixel uint16
	}
	var ws winsize
	for _, fd := range []int{int(os.Stdout.Fd()), int(os.Stderr.Fd()), int(os.Stdin.Fd())} {
		_, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL,
			uintptr(fd),
			uintptr(syscall.TIOCGWINSZ),
			uintptr(unsafe.Pointer(&ws)),
		)
		if errno == 0 && ws.Col > 0 && ws.Row > 0 {
			return TerminalSize{
				Cols:     int(ws.Col),
				Rows:     int(ws.Row),
				WidthPx:  int(ws.Xpixel),
				HeightPx: int(ws.Ypixel),
			}
		}
	}
	return TerminalSize{Cols: 80, Rows: 24, WidthPx: 0, HeightPx: 0}
}

func ComputePlacement(term TerminalSize, canvasW, canvasH int) Placement {
	if term.Cols <= 0 {
		term.Cols = 80
	}
	if term.Rows <= 0 {
		term.Rows = 24
	}
	if canvasW <= 0 || canvasH <= 0 {
		return Placement{WidthPx: canvasW, HeightPx: canvasH, StartCol: 1, StartRow: 1}
	}

	termWPx := float64(term.WidthPx)
	termHPx := float64(term.HeightPx)
	if term.WidthPx <= 0 || term.HeightPx <= 0 {
		// Fallback cell size if pixel query is unsupported
		termWPx = float64(term.Cols * 10)
		termHPx = float64(term.Rows * 20)
	}

	cellW := termWPx / float64(term.Cols)
	cellH := termHPx / float64(term.Rows)

	scaleX := termWPx / float64(canvasW)
	scaleY := termHPx / float64(canvasH)
	scale := math.Min(scaleX, scaleY)

	// Snap near-integer scale to preserve crisp pixels
	if math.Abs(scale-math.Round(scale)) < 0.04 {
		scale = math.Round(scale)
	}
	if scale < 0.01 {
		scale = 0.01
	}

	dispW := int(math.Round(float64(canvasW) * scale))
	dispH := int(math.Round(float64(canvasH) * scale))

	startCol := 1 + int(math.Floor((termWPx-float64(dispW))/(2.0*cellW)))
	startRow := 1 + int(math.Floor((termHPx-float64(dispH))/(2.0*cellH)))
	if startCol < 1 {
		startCol = 1
	}
	if startRow < 1 {
		startRow = 1
	}

	return Placement{
		WidthPx:  dispW,
		HeightPx: dispH,
		StartCol: startCol,
		StartRow: startRow,
	}
}

func Setup(w io.Writer) {
	_, _ = io.WriteString(w, "\x1b[?1049h\x1b[?25l")
}

func Restore(w io.Writer) {
	_, _ = io.WriteString(w, "\x1b_Ga=d\x1b\\\x1b[?25h\x1b[?1049l")
}

type Presenter struct {
	tempDir      string
	b64Buf       []byte
	prevFilePath string
	dispImg      *image.RGBA
}

func NewPresenter() *Presenter {
	tempDir := "/dev/shm"
	if fi, err := os.Stat(tempDir); err != nil || !fi.IsDir() {
		tempDir = os.TempDir()
	}
	return &Presenter{
		tempDir: tempDir,
	}
}

func ScaleNearest(src, dst *image.RGBA) {
	srcW := src.Bounds().Dx()
	srcH := src.Bounds().Dy()
	dstW := dst.Bounds().Dx()
	dstH := dst.Bounds().Dy()
	srcStride := src.Stride
	dstStride := dst.Stride
	srcPix := src.Pix
	dstPix := dst.Pix

	for dy := 0; dy < dstH; dy++ {
		sy := (dy * srcH) / dstH
		srcRow := sy * srcStride
		dstRow := dy * dstStride
		for dx := 0; dx < dstW; dx++ {
			sx := (dx * srcW) / dstW
			sp := srcRow + sx*4
			dp := dstRow + dx*4
			copy(dstPix[dp:dp+4], srcPix[sp:sp+4])
		}
	}
}

func (p *Presenter) SendFrame(w io.Writer, img *image.RGBA, imgID uint32, place Placement) error {
	targetImg := img
	if place.WidthPx > 0 && place.HeightPx > 0 {
		srcW := img.Bounds().Dx()
		srcH := img.Bounds().Dy()
		if place.WidthPx != srcW || place.HeightPx != srcH {
			if p.dispImg == nil || p.dispImg.Bounds().Dx() != place.WidthPx || p.dispImg.Bounds().Dy() != place.HeightPx {
				p.dispImg = image.NewRGBA(image.Rect(0, 0, place.WidthPx, place.HeightPx))
			}
			ScaleNearest(img, p.dispImg)
			targetImg = p.dispImg
		}
	}

	if p.tempDir != "" {
		if err := p.sendTempFile(w, targetImg, imgID, place); err == nil {
			return nil
		}
	}
	return p.sendDirect(w, targetImg, imgID, place)
}

func (p *Presenter) sendTempFile(w io.Writer, img *image.RGBA, imgID uint32, place Placement) error {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	bytesNeeded := width * height * 4

	cnt := atomic.AddUint64(&tempFileCounter, 1)
	filename := fmt.Sprintf("tty-graphics-protocol-%d-%d.bin", os.Getpid(), cnt)
	filePath := filepath.Join(p.tempDir, filename)

	if err := os.WriteFile(filePath, img.Pix[:bytesNeeded], 0600); err != nil {
		return err
	}

	if p.prevFilePath != "" && p.prevFilePath != filePath {
		_ = os.Remove(p.prevFilePath)
	}
	p.prevFilePath = filePath

	encodedPath := base64.StdEncoding.EncodeToString([]byte(filePath))

	posCmd := "\x1b[H"
	if place.StartRow > 1 || place.StartCol > 1 {
		posCmd = fmt.Sprintf("\x1b[%d;%dH", place.StartRow, place.StartCol)
	}

	// Omit c and r for 1:1 pixel mapping without GPU bilinear blur
	cmd := fmt.Sprintf("%s\x1b_Ga=T,f=32,s=%d,v=%d,t=t,C=1,q=2,i=%d;%s\x1b\\",
		posCmd, width, height, imgID, encodedPath)

	_, err := io.WriteString(w, cmd)
	return err
}

func (p *Presenter) sendDirect(w io.Writer, img *image.RGBA, imgID uint32, place Placement) error {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	data := img.Pix[:width*height*4]

	encodedLen := base64.StdEncoding.EncodedLen(len(data))
	if len(p.b64Buf) < encodedLen {
		p.b64Buf = make([]byte, encodedLen)
	}
	base64.StdEncoding.Encode(p.b64Buf, data)
	payload := p.b64Buf[:encodedLen]

	posCmd := "\x1b[H"
	if place.StartRow > 1 || place.StartCol > 1 {
		posCmd = fmt.Sprintf("\x1b[%d;%dH", place.StartRow, place.StartCol)
	}
	if _, err := io.WriteString(w, posCmd); err != nil {
		return err
	}

	const chunkSize = 4096
	totalLen := len(payload)
	offset := 0
	isFirst := true

	for offset < totalLen {
		end := offset + chunkSize
		m := 1
		if end >= totalLen {
			end = totalLen
			m = 0
		}

		chunk := payload[offset:end]

		if isFirst {
			// Omit c and r for 1:1 pixel mapping without GPU bilinear blur
			header := "\x1b_Ga=T,f=32,s=" + strconv.Itoa(width) +
				",v=" + strconv.Itoa(height) +
				",C=1,q=2,i=" + strconv.FormatUint(uint64(imgID), 10) +
				",m=" + strconv.Itoa(m) + ";"
			if _, err := io.WriteString(w, header); err != nil {
				return err
			}
			isFirst = false
		} else {
			header := "\x1b_Gm=" + strconv.Itoa(m) + ";"
			if _, err := io.WriteString(w, header); err != nil {
				return err
			}
		}

		if _, err := w.Write(chunk); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\x1b\\"); err != nil {
			return err
		}

		offset = end
	}

	return nil
}

func (p *Presenter) DeleteImage(w io.Writer, imgID uint32) {
	cmd := fmt.Sprintf("\x1b_Ga=d,d=i,i=%d\x1b\\", imgID)
	_, _ = io.WriteString(w, cmd)
}

func (p *Presenter) Cleanup() {
	if p.prevFilePath != "" {
		_ = os.Remove(p.prevFilePath)
		p.prevFilePath = ""
	}
}
