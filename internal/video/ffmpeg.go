package video

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type Reader struct {
	Width  int
	Height int
	FPS    float64

	cmd    *exec.Cmd
	stdout io.ReadCloser
}

type ffprobeStream struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	RFrameRate string `json:"r_frame_rate"`
	AvgFPS     string `json:"avg_frame_rate"`
}

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
}

func Open(filePath string, targetFPS float64) (*Reader, error) {
	probeCmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "v:0",
		filePath,
	)

	out, err := probeCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed on %s: %w", filePath, err)
	}

	var data ffprobeOutput
	if err := json.Unmarshal(out, &data); err != nil || len(data.Streams) == 0 {
		return nil, fmt.Errorf("no video stream found in %s", filePath)
	}

	stream := data.Streams[0]
	fps := parseFPS(stream.RFrameRate)
	if fps <= 0 {
		fps = parseFPS(stream.AvgFPS)
	}
	if targetFPS > 0 {
		fps = targetFPS
	} else if fps <= 0 {
		fps = 30.0
	}

	args := []string{
		"-v", "error",
		"-i", filePath,
		"-r", fmt.Sprintf("%.2f", fps),
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-",
	}

	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open ffmpeg stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	return &Reader{
		Width:  stream.Width,
		Height: stream.Height,
		FPS:    fps,
		cmd:    cmd,
		stdout: stdout,
	}, nil
}

func parseFPS(s string) float64 {
	parts := strings.Split(s, "/")
	if len(parts) == 1 {
		v, _ := strconv.ParseFloat(parts[0], 64)
		return v
	}
	if len(parts) == 2 {
		num, err1 := strconv.ParseFloat(parts[0], 64)
		den, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 == nil && err2 == nil && den > 0 {
			return num / den
		}
	}
	return 0
}

func (r *Reader) ReadFrame(dst *image.RGBA) error {
	expectedBytes := r.Width * r.Height * 4
	if len(dst.Pix) < expectedBytes {
		dst.Pix = make([]uint8, expectedBytes)
	}
	_, err := io.ReadFull(r.stdout, dst.Pix[:expectedBytes])
	return err
}

func (r *Reader) Close() error {
	if r.stdout != nil {
		_ = r.stdout.Close()
	}
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
		_ = r.cmd.Wait()
	}
	return nil
}
