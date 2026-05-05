// render.go — terminal half-block renderer for chewsprite.
//
// Each terminal cell holds 2 stacked pixels via the upper-half-block
// character (▀): foreground color = top pixel, background color = bottom
// pixel. With truecolor escapes (\033[38;2;R;G;Bm / \033[48;2;R;G;Bm) we
// get the full NES palette through. In standard 1:2-aspect-ratio monospace
// fonts, each half-block pixel renders as approximately square.
//
// Transparent pixels (palette index 0) can either inherit the terminal's
// default background or render as a configurable color.

package chewsprite

import (
	"fmt"
	"image/color"
	"strings"
)

const (
	upperHalfBlock = "▀"
	// pixelCell: two spaces with background color only. Each rendered "pixel"
	// is a fully-filled terminal cell (no glyph), so font line-gaps fall
	// between pixel rows instead of slicing through them. Aspect ratio is
	// near-square in standard 1:2 monospace fonts (2 char-widths ≈ 1 line-height).
	pixelCell = "  "
)

// RenderOptions controls how transparent pixels are rendered.
type RenderOptions struct {
	// TransparentBg: if non-nil, transparent pixels render with this RGB.
	// nil means use the terminal's default fg/bg (transparent).
	TransparentBg *[3]uint8
}

// RenderFullCell returns a multi-line string for the given frame using ONE
// fully-filled terminal cell per NES pixel (2 chars wide × 1 line tall,
// background color only, no glyph). This eliminates the line-gap stripes
// that show up with half-block (▀) rendering — each pixel is contained
// entirely within a single line, so font line-leading falls between pixel
// rows instead of through them.
//
// palette is the sprite's 4-color table (index 0 = transparent).
//
// Output dimensions per 16x16 frame: 32 chars wide × 16 lines tall.
func RenderFullCell(frame Frame, palette [4]color.RGBA, opts RenderOptions) string {
	var b strings.Builder
	for y := 0; y < FramePixels; y++ {
		for x := 0; x < FramePixels; x++ {
			writeBGFromPalette(&b, frame[y][x], palette, opts)
			b.WriteString(pixelCell)
		}
		b.WriteString("\033[0m\n")
	}
	return b.String()
}

// RenderFullCellByIndex renders the canonical CHEW sprite by frame index.
func RenderFullCellByIndex(frame int, opts RenderOptions) string {
	return RenderFullCell(DecodeFrame(frame), Palette, opts)
}

// RenderGumFullCellByIndex renders the GUM sprite by frame index.
func RenderGumFullCellByIndex(frame int, opts RenderOptions) string {
	return RenderFullCell(DecodeGumFrame(frame), GumPalette, opts)
}

// RenderHalfBlock returns a multi-line string for the given frame using
// upper-half-blocks with truecolor. The string ends with a reset+newline
// per row and a final ANSI reset.
func RenderHalfBlock(frame int, opts RenderOptions) string {
	grid := DecodeFrame(frame)
	var b strings.Builder
	for y := 0; y < FramePixels; y += 2 {
		for x := 0; x < FramePixels; x++ {
			top := grid[y][x]
			bot := grid[y+1][x]
			writeFG(&b, top, opts)
			writeBG(&b, bot, opts)
			b.WriteString(upperHalfBlock)
		}
		b.WriteString("\033[0m\n")
	}
	return b.String()
}

func writeFG(b *strings.Builder, idx uint8, opts RenderOptions) {
	if idx == 0 {
		if opts.TransparentBg != nil {
			c := *opts.TransparentBg
			fmt.Fprintf(b, "\033[38;2;%d;%d;%dm", c[0], c[1], c[2])
		} else {
			b.WriteString("\033[39m")
		}
		return
	}
	c := PaletteColor(idx)
	fmt.Fprintf(b, "\033[38;2;%d;%d;%dm", c.R, c.G, c.B)
}

func writeBG(b *strings.Builder, idx uint8, opts RenderOptions) {
	writeBGFromPalette(b, idx, Palette, opts)
}

func writeBGFromPalette(b *strings.Builder, idx uint8, palette [4]color.RGBA, opts RenderOptions) {
	if idx == 0 {
		if opts.TransparentBg != nil {
			c := *opts.TransparentBg
			fmt.Fprintf(b, "\033[48;2;%d;%d;%dm", c[0], c[1], c[2])
		} else {
			b.WriteString("\033[49m")
		}
		return
	}
	c := palette[idx]
	fmt.Fprintf(b, "\033[48;2;%d;%d;%dm", c.R, c.G, c.B)
}
