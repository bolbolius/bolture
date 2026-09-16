package render

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

var namedColors = map[string]color.RGBA{
	"transparent": {0, 0, 0, 0},
	"none":        {0, 0, 0, 0},
	"clear":       {0, 0, 0, 0},
	"black":       {0, 0, 0, 255},
	"white":       {255, 255, 255, 255},
	"red":         {255, 0, 0, 255},
	"green":       {0, 255, 0, 255},
	"blue":        {0, 0, 255, 255},
	"yellow":      {255, 255, 0, 255},
	"cyan":        {0, 255, 255, 255},
	"magenta":     {255, 0, 255, 255},
	"gray":        {128, 128, 128, 255},
	"grey":        {128, 128, 128, 255},
	"amber":       {255, 176, 0, 255},
	"matrix":      {0, 255, 65, 255},
	"stipple":     {240, 216, 192, 255},
}

func ParseColor(s string) (color.RGBA, error) {
	clean := strings.ToLower(strings.TrimSpace(s))
	if clean == "" {
		return color.RGBA{}, fmt.Errorf("empty color string")
	}

	if c, ok := namedColors[clean]; ok {
		return c, nil
	}

	clean = strings.TrimPrefix(clean, "#")

	switch len(clean) {
	case 3:
		r, err := strconv.ParseUint(string([]byte{clean[0], clean[0]}), 16, 8)
		if err != nil {
			return color.RGBA{}, err
		}
		g, err := strconv.ParseUint(string([]byte{clean[1], clean[1]}), 16, 8)
		if err != nil {
			return color.RGBA{}, err
		}
		b, err := strconv.ParseUint(string([]byte{clean[2], clean[2]}), 16, 8)
		if err != nil {
			return color.RGBA{}, err
		}
		return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil

	case 6:
		v, err := strconv.ParseUint(clean, 16, 32)
		if err != nil {
			return color.RGBA{}, err
		}
		return color.RGBA{
			R: uint8((v >> 16) & 0xFF),
			G: uint8((v >> 8) & 0xFF),
			B: uint8(v & 0xFF),
			A: 255,
		}, nil

	case 8:
		v, err := strconv.ParseUint(clean, 16, 32)
		if err != nil {
			return color.RGBA{}, err
		}
		return color.RGBA{
			R: uint8((v >> 24) & 0xFF),
			G: uint8((v >> 16) & 0xFF),
			B: uint8((v >> 8) & 0xFF),
			A: uint8(v & 0xFF),
		}, nil

	default:
		return color.RGBA{}, fmt.Errorf("invalid color format: %q", s)
	}
}
