package main

import (
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bolture/internal/kitty"
	"bolture/internal/render"
	"bolture/internal/video"
)

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: bolture [options] <input.mp4>

Play stylized stippled video art inside the Kitty terminal or export to file.

Options:
  --canvas WIDTHxHEIGHT   Render canvas dimensions (default: 1280x720)
  --dot-size SCALE        Relative visual dot scale (default: 1.0)
  --density MULT          Dot density multiplier (default: 1.0)
  --temporal-stability VALUE Temporal stability: off, low, medium, high, or 0.0-1.0 (default: medium)
  --fps FPS               Framerate for playback and export (default: 30 or video fps)
  --fullscreen            Scale display to fill terminal window (default: 1:1 pixel mapping)
  --loop                  Loop playback when the video ends
  --dither METHOD         Dithering algorithm: atkinson or bayer (default: atkinson)
  --foreground COLOR      Foreground dot color (hex or named, default: #f0d8c0)
  --background COLOR      Background color (hex, named, or transparent, default: transparent)
  -o, --output FILE       Output file: image (.png, .jpg) or video (.mp4, .mkv, .mov, .webm)
`)
}

func main() {
	var (
		canvasFlag            string
		dotSizeFlag           float64
		densityFlag           float64
		temporalStabilityFlag string
		fpsFlag               float64
		fullscreenFlag        bool
		loopFlag              bool
		ditherFlag            string
		fgFlag                string
		bgFlag                string
		outFlag               string
		outShortFlag          string
	)

	fs := flag.NewFlagSet("bolture", flag.ExitOnError)
	fs.StringVar(&canvasFlag, "canvas", "1280x720", "Render canvas dimensions (e.g. 1280x720)")
	fs.Float64Var(&dotSizeFlag, "dot-size", 1.0, "Relative visual dot scale (default: 1.0)")
	fs.Float64Var(&densityFlag, "density", 1.0, "Dot density multiplier")
	fs.StringVar(&temporalStabilityFlag, "temporal-stability", "medium", "Temporal stability: off, low, medium, high, or 0.0-1.0 (default: medium)")
	fs.Float64Var(&fpsFlag, "fps", 0, "Framerate for playback and export (default: 30 or video fps)")
	fs.BoolVar(&fullscreenFlag, "fullscreen", false, "Scale display to fill terminal window (default: 1:1 pixel mapping)")
	fs.BoolVar(&loopFlag, "loop", false, "Loop playback when the video ends")
	fs.StringVar(&ditherFlag, "dither", "atkinson", "Dithering algorithm: atkinson or bayer")
	fs.StringVar(&fgFlag, "foreground", "#f0d8c0", "Foreground dot color")
	fs.StringVar(&bgFlag, "background", "transparent", "Background color (or transparent)")
	fs.StringVar(&outFlag, "output", "", "Output file (image or video)")
	fs.StringVar(&outShortFlag, "o", "", "Output file (shorthand)")

	fs.Usage = printUsage

	flagArgs, inputPath := splitArgs(os.Args[1:])

	if err := fs.Parse(flagArgs); err != nil {
		printUsage()
		os.Exit(1)
	}

	if inputPath == "" {
		printUsage()
		os.Exit(1)
	}

	width, height, err := parseCanvas(canvasFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fg, err := render.ParseColor(fgFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid foreground color %q: %v\n", fgFlag, err)
		os.Exit(1)
	}

	bg, err := render.ParseColor(bgFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid background color %q: %v\n", bgFlag, err)
		os.Exit(1)
	}

	temporalStability, err := parseTemporalStability(temporalStabilityFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	outputPath := outFlag
	if outputPath == "" {
		outputPath = outShortFlag
	}

	reader, err := video.Open(inputPath, fpsFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open video: %v\n", err)
		os.Exit(1)
	}
	defer reader.Close()

	renderer := render.NewRenderer(render.Options{
		Width:             width,
		Height:            height,
		DotSize:           dotSizeFlag,
		Density:           densityFlag,
		TemporalStability: temporalStability,
		Dither:            strings.ToLower(ditherFlag),
		Foreground:        fg,
		Background:        bg,
	})

	srcFrame := image.NewRGBA(image.Rect(0, 0, reader.Width, reader.Height))
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))

	if outputPath != "" {
		ext := strings.ToLower(filepath.Ext(outputPath))
		switch ext {
		case ".mp4", ".mkv", ".mov", ".webm", ".avi":
			var encArgs []string
			if ext == ".webm" {
				encArgs = []string{
					"-y", "-f", "rawvideo", "-pix_fmt", "rgba",
					"-s", fmt.Sprintf("%dx%d", width, height),
					"-r", fmt.Sprintf("%.2f", reader.FPS),
					"-i", "-",
					"-c:v", "libvpx-vp9",
					"-crf", "30",
					"-b:v", "0",
					outputPath,
				}
			} else {
				encArgs = []string{
					"-y", "-f", "rawvideo", "-pix_fmt", "rgba",
					"-s", fmt.Sprintf("%dx%d", width, height),
					"-r", fmt.Sprintf("%.2f", reader.FPS),
					"-i", "-",
					"-c:v", "libx264",
					"-pix_fmt", "yuv420p",
					"-crf", "18",
					"-preset", "fast",
					outputPath,
				}
			}

			ffmpegCmd := exec.Command("ffmpeg", encArgs...)
			stdinPipe, err := ffmpegCmd.StdinPipe()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create ffmpeg stdin: %v\n", err)
				os.Exit(1)
			}
			ffmpegCmd.Stderr = os.Stderr
			if err := ffmpegCmd.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to start ffmpeg: %v\n", err)
				os.Exit(1)
			}

			fmt.Fprintf(os.Stderr, "Rendering video to %s (%dx%d @ %.2f fps)...\n", outputPath, width, height, reader.FPS)
			frameNum := 0
			for {
				if err := reader.ReadFrame(srcFrame); err != nil {
					break
				}
				if err := renderer.Render(srcFrame, canvas); err != nil {
					fmt.Fprintf(os.Stderr, "Render error on frame %d: %v\n", frameNum, err)
					break
				}
				if _, err := stdinPipe.Write(canvas.Pix); err != nil {
					break
				}
				frameNum++
				if frameNum%30 == 0 {
					fmt.Fprintf(os.Stderr, "\rRendered frame %d...", frameNum)
				}
			}
			stdinPipe.Close()
			if err := ffmpegCmd.Wait(); err != nil {
				fmt.Fprintf(os.Stderr, "\nffmpeg encoding failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "\nSuccessfully rendered %d frames to %s\n", frameNum, outputPath)
			return

		default:
			if err := reader.ReadFrame(srcFrame); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read video frame: %v\n", err)
				os.Exit(1)
			}
			if err := renderer.Render(srcFrame, canvas); err != nil {
				fmt.Fprintf(os.Stderr, "Render error: %v\n", err)
				os.Exit(1)
			}
			f, err := os.Create(outputPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create %s: %v\n", outputPath, err)
				os.Exit(1)
			}
			defer f.Close()

			if ext == ".jpg" || ext == ".jpeg" {
				if err := jpeg.Encode(f, canvas, &jpeg.Options{Quality: 95}); err != nil {
					fmt.Fprintf(os.Stderr, "JPEG encode failed: %v\n", err)
					os.Exit(1)
				}
			} else {
				if err := png.Encode(f, canvas); err != nil {
					fmt.Fprintf(os.Stderr, "PNG encode failed: %v\n", err)
					os.Exit(1)
				}
			}
			fmt.Fprintf(os.Stderr, "Rendered frame saved to %s\n", outputPath)
			return
		}
	}

	kitty.Setup(os.Stdout)
	presenter := kitty.NewPresenter()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		kitty.Restore(os.Stdout)
		presenter.Cleanup()
		os.Exit(0)
	}()
	defer func() {
		kitty.Restore(os.Stdout)
		presenter.Cleanup()
	}()

	frameDuration := time.Duration(float64(time.Second) / reader.FPS)
	startTime := time.Now()
	frameIndex := int64(0)
	imgID := uint32(1)

	winchChan := make(chan os.Signal, 1)
	signal.Notify(winchChan, syscall.SIGWINCH)

	curPlacement := kitty.ComputePlacement(kitty.GetWindowSize(), width, height)

playback:
	for {
		now := time.Now()
		elapsed := now.Sub(startTime)
		expectedFrame := int64(elapsed / frameDuration)

		// Drop frames if falling behind
		if expectedFrame > frameIndex+1 {
			for frameIndex < expectedFrame {
				if err := reader.ReadFrame(srcFrame); err != nil {
					if !loopFlag {
						return
					}
					if err := reader.Restart(); err != nil {
						return
					}
					renderer.Reset()
					startTime = time.Now()
					frameIndex = 0
					continue playback
				}
				frameIndex++
			}
		}

		if err := reader.ReadFrame(srcFrame); err != nil {
			if !loopFlag {
				return
			}
			if err := reader.Restart(); err != nil {
				return
			}
			renderer.Reset()
			startTime = time.Now()
			frameIndex = 0
			continue
		}
		frameIndex++

		targetTime := startTime.Add(time.Duration(frameIndex) * frameDuration)
		now = time.Now()
		if targetTime.After(now) {
			time.Sleep(targetTime.Sub(now))
		}

		if err := renderer.Render(srcFrame, canvas); err != nil {
			return
		}

		select {
		case <-winchChan:
			curPlacement = kitty.ComputePlacement(kitty.GetWindowSize(), width, height)
			_, _ = io.WriteString(os.Stdout, "\x1b[2J")
		default:
		}

		nextID := imgID
		imgID = 3 - imgID
		if err := presenter.SendFrame(os.Stdout, canvas, nextID, curPlacement); err != nil {
			return
		}
		presenter.DeleteImage(os.Stdout, imgID)
	}
}

func parseCanvas(s string) (int, int, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid canvas format %q, expected WIDTHxHEIGHT (e.g. 1280x720)", s)
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("invalid canvas dimensions %q", s)
	}
	return w, h, nil
}

func parseTemporalStability(s string) (float64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "off", "none", "0":
		return 0.0, nil
	case "low":
		return 0.3, nil
	case "medium", "med", "default":
		return 0.6, nil
	case "high":
		return 0.85, nil
	default:
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v < 0.0 || v > 1.0 {
			return 0, fmt.Errorf("invalid --temporal-stability %q: must be off, low, medium, high, or float between 0.0 and 1.0", s)
		}
		return v, nil
	}
}

func splitArgs(rawArgs []string) (flagArgs []string, inputPath string) {
	for i := 0; i < len(rawArgs); i++ {
		arg := rawArgs[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			isBool := arg == "--fullscreen" || arg == "-fullscreen" || arg == "--loop" || arg == "-loop"
			if !isBool && !strings.Contains(arg, "=") && i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-") {
				flagArgs = append(flagArgs, rawArgs[i+1])
				i++
			}
		} else if inputPath == "" {
			inputPath = arg
		}
	}
	return flagArgs, inputPath
}
