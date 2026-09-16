package video

import (
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReader(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "test.mp4")

	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi",
		"-i", "testsrc=duration=0.5:size=160x120:rate=10",
		samplePath,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to generate test video: %v", err)
	}
	defer os.Remove(samplePath)

	r, err := Open(samplePath, 10.0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer r.Close()

	if r.Width != 160 || r.Height != 120 {
		t.Errorf("expected 160x120, got %dx%d", r.Width, r.Height)
	}

	frame := image.NewRGBA(image.Rect(0, 0, r.Width, r.Height))
	count := 0
	for {
		err := r.ReadFrame(frame)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ReadFrame failed at frame %d: %v", count, err)
		}
		count++
	}

	if count != 5 {
		t.Errorf("expected 5 frames, got %d", count)
	}
}
