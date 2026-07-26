package mcp

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// placeholderThumbPNG returns a simple solid-color PNG suitable as a library thumbnail
// when an agent upload does not supply one.
func placeholderThumbPNG() []byte {
	const w, h = 640, 400
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	bg := color.RGBA{R: 15, G: 118, B: 110, A: 255}      // teal
	accent := color.RGBA{R: 204, G: 251, B: 241, A: 255} // light teal

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := bg
			// Soft top gradient band
			if y < h/3 {
				t := float64(y) / float64(h/3)
				c = color.RGBA{
					R: uint8(float64(accent.R)*(1-t) + float64(bg.R)*t),
					G: uint8(float64(accent.G)*(1-t) + float64(bg.G)*t),
					B: uint8(float64(accent.B)*(1-t) + float64(bg.B)*t),
					A: 255,
				}
			}
			// Decorative bottom bar
			if y > h-24 {
				c = color.RGBA{R: 19, G: 78, B: 74, A: 255}
			}
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
