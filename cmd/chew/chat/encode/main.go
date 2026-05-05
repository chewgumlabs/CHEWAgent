// chew sprite encoder — converts an aseprite-exported PNG + JSON into NES
// CHR-ROM-format bit-plane bytes baked into a Go source file.
//
// Reads frame positions from the aseprite JSON; labels are supplied via
// --labels (comma-separated, must match frame count). Output package and
// variable prefix are flag-controlled so the same tool serves multiple
// sprites in the same Go package without symbol collisions.
//
// Examples:
//   # CHEW (8 frames):
//   go run ./cmd/chew/chat/encode \
//     --json cmd/chew/chat/assets/CHEW_NES.json \
//     --png  cmd/chew/chat/assets/CHEW_NES.png \
//     --out  cmd/chew/chat/sprite/chr_data.go \
//     --package chewsprite \
//     --var-prefix "" \
//     --labels "idle_0,idle_1,idle_2,walk_0,walk_1,walk_2,ghost_0,ghost_1"
//
//   # GUM (6 frames):
//   go run ./cmd/chew/chat/encode \
//     --json cmd/chew/chat/assets/nesGUM.json \
//     --png  cmd/chew/chat/assets/nesGUM.png \
//     --out  cmd/chew/chat/sprite/gum_chr.go \
//     --package chewsprite \
//     --var-prefix "Gum" \
//     --labels "down_0,down_1,left_0,left_1,up_0,up_1"
//
// The CHR-ROM encoding follows the NES PPU's per-tile format exactly:
// 16 bytes per 8x8 tile = 8 bytes plane-0 (low bit per pixel) + 8 bytes
// plane-1 (high bit per pixel). For pixel (x,y) within a tile:
//   idx = ((plane1[y] >> (7-x)) & 1) << 1 | ((plane0[y] >> (7-x)) & 1)

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"sort"
	"strings"
)

const (
	frameSize     = 16
	tileSize      = 8
	tilesPerFrame = 4
	bytesPerTile  = 16
	bytesPerFrame = tilesPerFrame * bytesPerTile
)

// Aseprite JSON shape we care about.
type asepriteJSON struct {
	Frames []asepriteFrame `json:"frames"`
	Meta   struct {
		Size struct {
			W int `json:"w"`
			H int `json:"h"`
		} `json:"size"`
	} `json:"meta"`
}
type asepriteFrame struct {
	Filename string `json:"filename"`
	Frame    struct {
		X int `json:"x"`
		Y int `json:"y"`
		W int `json:"w"`
		H int `json:"h"`
	} `json:"frame"`
}

var tileOffsets = [tilesPerFrame][2]int{
	{0, 0}, {8, 0}, {0, 8}, {8, 8}, // TL, TR, BL, BR
}

func main() {
	var (
		jsonPath  = flag.String("json", "", "aseprite JSON describing frame positions (required)")
		pngPath   = flag.String("png", "", "PNG sprite sheet (required)")
		outPath   = flag.String("out", "", "output Go source path (required)")
		pkgName   = flag.String("package", "chewsprite", "Go package name for output")
		varPrefix = flag.String("var-prefix", "", "prefix for generated identifiers (e.g. 'Gum' -> GumPalette, GumCHR, GumFrameLabels)")
		labelsCSV = flag.String("labels", "", "comma-separated frame labels; must match frame count from JSON (required)")
	)
	flag.Parse()
	if *jsonPath == "" || *pngPath == "" || *outPath == "" || *labelsCSV == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\nError: --json, --png, --out, --labels are all required.")
		os.Exit(2)
	}

	asepriteData, err := loadAseprite(*jsonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load %s: %v\n", *jsonPath, err)
		os.Exit(1)
	}

	// Validate frame dimensions
	for i, fr := range asepriteData.Frames {
		if fr.Frame.W != frameSize || fr.Frame.H != frameSize {
			fmt.Fprintf(os.Stderr, "frame %d (%s): expected %dx%d, got %dx%d\n",
				i, fr.Filename, frameSize, frameSize, fr.Frame.W, fr.Frame.H)
			os.Exit(1)
		}
	}

	labels := strings.Split(*labelsCSV, ",")
	for i := range labels {
		labels[i] = strings.TrimSpace(labels[i])
	}
	if len(labels) != len(asepriteData.Frames) {
		fmt.Fprintf(os.Stderr, "label count (%d) != frame count (%d)\n", len(labels), len(asepriteData.Frames))
		os.Exit(1)
	}

	img, err := loadPNG(*pngPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load %s: %v\n", *pngPath, err)
		os.Exit(1)
	}

	// Detect palette across the whole sprite
	palette, err := detectPalette(img)
	if err != nil {
		fmt.Fprintf(os.Stderr, "palette: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("detected palette (sprite %s):\n", *pngPath)
	for i, c := range palette {
		role := "color"
		if i == 0 {
			role = "transparent"
		}
		fmt.Printf("  [%d] %s  R=%#02x G=%#02x B=%#02x A=%#02x\n", i, role, c.R, c.G, c.B, c.A)
	}

	numFrames := len(asepriteData.Frames)
	allCHR := make([]byte, numFrames*bytesPerFrame)
	for i, fr := range asepriteData.Frames {
		framePos := [2]int{fr.Frame.X, fr.Frame.Y}
		frameCHR := encodeFrame(img, framePos, palette)
		copy(allCHR[i*bytesPerFrame:(i+1)*bytesPerFrame], frameCHR[:])
	}

	if err := writeGoSource(*outPath, *pkgName, *varPrefix, labels, allCHR, palette); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *outPath, err)
		os.Exit(1)
	}

	totalBytes := numFrames * bytesPerFrame
	fmt.Printf("encoded %d frames (%d bytes total) -> %s\n", numFrames, totalBytes, *outPath)
}

func loadAseprite(path string) (*asepriteJSON, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data asepriteJSON
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if len(data.Frames) == 0 {
		return nil, fmt.Errorf("no frames in JSON")
	}
	return &data, nil
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func toRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

type colorCount struct {
	c     color.RGBA
	count int
}

func detectPalette(img image.Image) ([4]color.RGBA, error) {
	bounds := img.Bounds()
	counts := map[color.RGBA]int{}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := toRGBA(img.At(x, y))
			if c.A < 128 {
				c = color.RGBA{0, 0, 0, 0}
			}
			counts[c]++
		}
	}

	transparent := color.RGBA{0, 0, 0, 0}
	var opaque []colorCount
	for c, n := range counts {
		if c.A < 128 {
			continue
		}
		opaque = append(opaque, colorCount{c, n})
	}
	if len(opaque) == 0 {
		return [4]color.RGBA{}, fmt.Errorf("no opaque pixels found")
	}
	if len(opaque) > 3 {
		var found []string
		for _, oc := range opaque {
			found = append(found, fmt.Sprintf("#%02x%02x%02x×%d", oc.c.R, oc.c.G, oc.c.B, oc.count))
		}
		return [4]color.RGBA{}, fmt.Errorf("expected ≤3 opaque colors, found %d: %s", len(opaque), strings.Join(found, ", "))
	}

	sort.Slice(opaque, func(i, j int) bool { return opaque[i].count > opaque[j].count })
	pal := [4]color.RGBA{transparent}
	for i, oc := range opaque {
		if i+1 >= 4 {
			break
		}
		pal[i+1] = oc.c
	}
	return pal, nil
}

func colorIndex(c color.RGBA, palette [4]color.RGBA) int {
	if c.A < 128 {
		return 0
	}
	for i := 1; i < 4; i++ {
		p := palette[i]
		if p.R == c.R && p.G == c.G && p.B == c.B {
			return i
		}
	}
	return 0
}

func encodeFrame(img image.Image, framePos [2]int, palette [4]color.RGBA) [bytesPerFrame]byte {
	var out [bytesPerFrame]byte
	for ti, t := range tileOffsets {
		tileBase := ti * bytesPerTile
		for row := 0; row < tileSize; row++ {
			var plane0, plane1 byte
			for col := 0; col < tileSize; col++ {
				px := framePos[0] + t[0] + col
				py := framePos[1] + t[1] + row
				idx := colorIndex(toRGBA(img.At(px, py)), palette)
				bit := byte(7 - col)
				if idx&1 != 0 {
					plane0 |= 1 << bit
				}
				if idx&2 != 0 {
					plane1 |= 1 << bit
				}
			}
			out[tileBase+row] = plane0
			out[tileBase+8+row] = plane1
		}
	}
	return out
}

func writeGoSource(path, pkgName, prefix string, labels []string, chr []byte, palette [4]color.RGBA) error {
	numFrames := len(labels)
	var b strings.Builder

	fmt.Fprintf(&b, `// Code generated by cmd/chew/chat/encode. DO NOT EDIT BY HAND.
// Format: NES CHR-ROM bit-plane (per 8x8 tile: 8 bytes plane-0 + 8 bytes plane-1).
// 16x16 frames are 4 tiles arranged TL, TR, BL, BR (2x2). %d frames total.

package %s

import "image/color"

`, numFrames, pkgName)

	// Constants — only the canonical sprite (no prefix) defines the shared
	// shape constants used by the decoder/renderer. Prefixed sprites assume
	// the package already has them.
	if prefix == "" {
		fmt.Fprintf(&b, `const (
	NumFrames     = %d
	FramePixels   = 16
	TilePixels    = 8
	TilesPerFrame = 4
	BytesPerTile  = 16
	BytesPerFrame = TilesPerFrame * BytesPerTile
)

`, numFrames)
	} else {
		// Per-sprite constant for frame count (the shared NumFrames is
		// scoped to the canonical sprite, others may differ).
		fmt.Fprintf(&b, "const %sNumFrames = %d\n\n", prefix, numFrames)
	}

	// FrameLabels
	fmt.Fprintf(&b, "// %sFrameLabels maps frame index to its animation slot name.\n", prefix)
	if prefix == "" {
		fmt.Fprintf(&b, "var FrameLabels = [NumFrames]string{\n")
	} else {
		fmt.Fprintf(&b, "var %sFrameLabels = [%sNumFrames]string{\n", prefix, prefix)
	}
	for _, l := range labels {
		fmt.Fprintf(&b, "\t%q,\n", l)
	}
	b.WriteString("}\n\n")

	// Palette
	fmt.Fprintf(&b, "// %sPalette: index 0 = transparent; 1..3 = colors picked from the source PNG.\n", prefix)
	fmt.Fprintf(&b, "var %sPalette = [4]color.RGBA{\n", prefix)
	for i, c := range palette {
		role := ""
		if i == 0 {
			role = " // transparent"
		}
		fmt.Fprintf(&b, "\t{R: 0x%02x, G: 0x%02x, B: 0x%02x, A: 0x%02x},%s\n", c.R, c.G, c.B, c.A, role)
	}
	b.WriteString("}\n\n")

	// CHR
	fmt.Fprintf(&b, "// %sCHR is the encoded sprite data: %d frames * 64 bytes each.\n", prefix, numFrames)
	if prefix == "" {
		fmt.Fprintf(&b, "var CHR = [NumFrames * BytesPerFrame]byte{\n")
	} else {
		fmt.Fprintf(&b, "var %sCHR = [%sNumFrames * BytesPerFrame]byte{\n", prefix, prefix)
	}
	for i := 0; i < numFrames; i++ {
		fmt.Fprintf(&b, "\t// frame %d (%s)\n", i, labels[i])
		for tile := 0; tile < tilesPerFrame; tile++ {
			b.WriteString("\t")
			for byteIdx := 0; byteIdx < bytesPerTile; byteIdx++ {
				offset := i*bytesPerFrame + tile*bytesPerTile + byteIdx
				fmt.Fprintf(&b, "0x%02x, ", chr[offset])
			}
			tileNames := []string{"TL", "TR", "BL", "BR"}
			fmt.Fprintf(&b, "// tile %s\n", tileNames[tile])
		}
	}
	b.WriteString("}\n")
	return os.WriteFile(path, []byte(b.String()), 0644)
}
