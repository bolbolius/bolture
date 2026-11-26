# bolture

**Dithered video for the terminal.**

`bolture` is a small Go CLI that converts video into sharp, dithered dot patterns and plays them directly in the terminal using the Kitty Graphics Protocol.

It can also export the rendered result as PNG, JPEG, MP4, WebM, MKV, or MOV.

> Terminal playback requires Kitty Graphics Protocol support (kitty, Ghostty, WezTerm). File export works independently of terminal graphics support.

## Demo

https://github.com/user-attachments/assets/855c27ac-1064-4fab-8683-f3afa4923fc2

<img width="793" height="834" alt="Image" src="https://github.com/user-attachments/assets/c5d2c9aa-bbbe-44ce-91e6-c907023d1261" />

## Install

Tagged releases include Linux binaries for amd64 and arm64. They are on the [releases page](https://github.com/bolbolius/bolture/releases).

If you have Go 1.27.1 or newer:

```bash
go install github.com/bolbolius/bolture/cmd/bolture@latest
```

From source:

```bash
git clone https://github.com/bolbolius/bolture
cd bolture
go build -o bolture ./cmd/bolture
```

FFmpeg and FFprobe must be on `PATH`. bolture is pre-1.0, so flags and output formats can change.

## Usage

```bash
bolture [options] <input>
```

Play a video with default values:

```bash
bolture video.mp4
```

Use a larger canvas and dots:

```bash
bolture video.mp4 --canvas 1920x1080 --dot-size 3
```

Use Floyd-Steinberg dithering:

```bash
bolture video.mp4 --dither floyd-steinberg
```

Split sampling and drawing across four workers:

```bash
bolture video.mp4 --threads 4
```

`--threads 0` uses every CPU. The renderer runs on one core unless `--threads` is set.

Export a video:

```bash
bolture video.mp4 -o output.mp4 --fps 24
```

## Options

| Flag                         | Description                                        | Default       |
| ---------------------------- | -------------------------------------------------- | ------------- |
| `--canvas WIDTHxHEIGHT`      | Render resolution                                  | `1280x720`    |
| `--dot-size SCALE`           | Size of the dots                                   | `1.0`         |
| `--density MULT`             | Controls how much luminance is represented by dots | `1.0`         |
| `--temporal-stability VALUE` | `off`, `low`, `medium`, `high`, or `0.0` to `1.0`  | `medium`      |
| `--fps FPS`                  | Playback/export framerate                          | Source FPS    |
| `--fullscreen`               | Scale the frame to fill the terminal window        | `false`       |
| `--loop`                     | Loop playback when the video ends                  | `false`       |
| `--threads N`                | Sampling and draw workers; 0 uses every CPU        | `1`           |
| `--dither METHOD`            | See [Dithering algorithms](#dithering-algorithms) | `atkinson`    |
| `--foreground COLOR`         | Dot color                                          | `#f0d8c0`     |
| `--background COLOR`         | `transparent` or a color                           | `transparent` |
| `-o, --output FILE`          | Export to an image or video                        | none          |

## Dithering algorithms

Error diffusion spreads each pixel's error to neighbours. Wider kernels smooth gradients more, blur fine detail, and cost more per frame. Ordered dithering uses a fixed threshold matrix, which is faster and produces a stable pattern.

| Name | Notes |
| --- | --- |
| `atkinson` | Default. Lighter, sparser dots; throws away a quarter of the error. |
| `floyd-steinberg` | Classic kernel. Good detail, with some worm-like artifacts on flat areas. |
| `burkes` | Wider Floyd-Steinberg variant, smoother gradients. |
| `sierra` | Three-row kernel, smoother than Floyd-Steinberg with less blur than Stucki. |
| `stucki` | Wide kernel, sharp gradients. |
| `jarvis` | Five-column kernel, the smoothest of the diffusion kernels. |
| `bayer4` | Small ordered matrix, coarse pattern. |
| `bayer8` | Ordered matrix; `bayer` is an alias. |
| `bayer16` | Large ordered matrix, finer pattern. |

All kernels work with `--dot-size` and `--temporal-stability`.

## Multi-core

The renderer is single-core by default. `--threads N` splits luminance sampling and frame drawing across N workers, and `--threads 0` uses every CPU. Asking for more workers than the machine has CPUs is capped.

The dither scan stays on one core. Error diffusion depends on the previous pixel's error, so it cannot be split without changing the result. Sampling and drawing are row-independent, so they are the stages that split cleanly.

## Features

* Real-time video playback in the terminal
* Atkinson, Floyd-Steinberg, Burkes, Sierra, Stucki, and Jarvis error diffusion
* Bayer 4×4, 8×8, and 16×16 ordered dithering
* Adjustable dot size and density
* Temporal stability to reduce flickering
* Perceptual luminance to separate background and foreground
* Aspect-ratio fitting and terminal centering
* Video export
* Direct FFmpeg streaming
* Zero heap allocations in the per-frame renderer

## How it works

`bolture` does not use object detection, segmentation, or any edge detection. It derives the foreground from each pixel's luminance.

Frames are decoded by FFmpeg as raw RGBA, resized to the requested canvas, and converted to perceptual luminance using BT.709 weighting. Very dark pixels are removed with a small cutoff to keep shadows clean.

The luminance is then converted into dots with an error-diffusion kernel or a Bayer matrix. `--density` controls how much of the luminance range is represented by dots.

Temporal hysteresis keeps small frame-to-frame changes from constantly flipping individual dots. This reduces flickering without using frame blending or motion blur.

When downscaling, `bolture` uses area averaging instead of simply skipping source pixels, helping preserve detail and reduce aliasing. When upscaling, it uses direct pixel sampling.

The renderer is described in more detail in [Zero-allocation dithering in Go](https://bolbolius.is-a.dev/blog/zero-allocation-dithering-in-go/).

## Terminal rendering

Rendered frames are sent directly to the terminal through the Kitty Graphics Protocol.

The output is automatically fitted and centered. Resizing the terminal only changes the placement of the existing frame, so the video does not need to be rendered again.

Frames are double-buffered to prevent the previous frame from briefly disappearing when a new frame is displayed.

## Export

Video output is streamed directly into FFmpeg without creating an intermediate image sequence.

## License

GPL-3.0-or-later. See [LICENSE](LICENSE).
