# bolture

**Dithered video for the terminal.**

`bolture` is a small Go CLI that converts video into sharp, dithered dot patterns and plays them directly in the terminal using the Kitty Graphics Protocol.

It can also export the rendered result as PNG, JPEG, MP4, WebM, MKV, or MOV.

> Terminal playback requires Kitty Graphics Protocol support. File export works independently of terminal graphics support.

## Demo

https://github.com/user-attachments/assets/855c27ac-1064-4fab-8683-f3afa4923fc2

<img width="793" height="834" alt="Image" src="https://github.com/user-attachments/assets/c5d2c9aa-bbbe-44ce-91e6-c907023d1261" />

## Features

* Real-time video playback in the terminal
* Atkinson and Bayer 8×8 dithering
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

The luminance is then converted into dots using either **Atkinson error diffusion** or **Bayer 8×8 ordered dithering**. `--density` controls how much of the luminance range is represented by dots.

Temporal hysteresis keeps small frame-to-frame changes from constantly flipping individual dots. This reduces flickering without using frame blending or motion blur.

When downscaling, `bolture` uses area averaging instead of simply skipping source pixels, helping preserve detail and reduce aliasing. When upscaling, it uses direct pixel sampling.

## Terminal rendering

Rendered frames are sent directly to the terminal through the Kitty Graphics Protocol.

The output is automatically fitted and centered. Resizing the terminal only changes the placement of the existing frame, so the video does not need to be rendered again.

Frames are double-buffered to prevent the previous frame from briefly disappearing when a new frame is displayed.

## Export

Video output is streamed directly into FFmpeg without creating an intermediate image sequence.

## Installation

Requires **Go 1.20+**, **FFmpeg**, and **FFprobe**.

```bash
git clone https://github.com/bolbolius/bolture
cd bolture
go build -o bolture ./cmd/bolture
```

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

Use Bayer dithering:

```bash
bolture video.mp4 --dither bayer
```

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
| `--temporal-stability VALUE` | `off`, `low`, `medium`, `high`, or `0.0`–`1.0`     | `medium`      |
| `--fps FPS`                  | Playback/export framerate                          | Source FPS    |
| `--loop`                   | Loop playback when the video ends                  | `false`       |
| `--dither METHOD`            | `atkinson` or `bayer`                              | `atkinson`    |
| `--foreground COLOR`         | Dot color                                          | `#f0d8c0`     |
| `--background COLOR`         | `transparent` or a color                           | `transparent` |
| `-o, --output FILE`          | Export to an image or video                        | —             |

## License

GPL-3.0-or-later
