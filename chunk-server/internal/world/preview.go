package world

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	previewTileWidth    = 32
	previewTileHeight   = 16
	previewBlockHeight  = 16
	previewAmbientLight = 0.2
)

type blockPreview struct {
	localX  int
	localY  int
	localZ  int
	block   Block
	screenX int
	screenY int
}

// SaveChunkPreview renders an isometric preview PNG for the provided chunk.
func SaveChunkPreview(chunk *Chunk, outputDir string) error {
	if chunk == nil {
		return fmt.Errorf("chunk is nil")
	}

	dim := chunk.Dimensions()
	if dim.Width <= 0 || dim.Depth <= 0 || dim.Height <= 0 {
		return fmt.Errorf("invalid chunk dimensions: %+v", dim)
	}

	width := (dim.Width+dim.Depth)*previewTileWidth/2 + previewTileWidth
	height := (dim.Width+dim.Depth)*previewTileHeight/2 + dim.Height*previewBlockHeight + previewTileHeight
	img := image.NewNRGBA(image.Rect(0, 0, width, height))

	background := color.NRGBA{R: 10, G: 10, B: 18, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{background}, image.Point{}, draw.Src)

	blocks := collectPreviewBlocks(chunk)
	if len(blocks) == 0 {
		if err := ensurePreviewDir(outputDir); err != nil {
			return err
		}
		path := filepath.Join(outputDir, fmt.Sprintf("chunk_%d_%d.png", chunk.Key.X, chunk.Key.Y))
		file, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("create preview: %w", err)
		}
		defer file.Close()
		if err := png.Encode(file, img); err != nil {
			return fmt.Errorf("encode preview: %w", err)
		}
		return nil
	}

	sort.Slice(blocks, func(i, j int) bool {
		bi := blocks[i]
		bj := blocks[j]
		if bi.screenY == bj.screenY {
			if bi.screenX == bj.screenX {
				if bi.localZ == bj.localZ {
					if bi.localY == bj.localY {
						return bi.localX < bj.localX
					}
					return bi.localY > bj.localY
				}
				return bi.localZ < bj.localZ
			}
			return bi.screenX < bj.screenX
		}
		return bi.screenY < bj.screenY
	})

	offsetX := dim.Depth * previewTileWidth / 2
	offsetY := dim.Height * previewBlockHeight

	for _, info := range blocks {
		baseX := offsetX + info.screenX
		baseY := offsetY + info.screenY
		renderBlockPreview(img, baseX, baseY, info.block)
	}

	if err := ensurePreviewDir(outputDir); err != nil {
		return err
	}

	path := filepath.Join(outputDir, fmt.Sprintf("chunk_%d_%d.png", chunk.Key.X, chunk.Key.Y))
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create preview: %w", err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		return fmt.Errorf("encode preview: %w", err)
	}
	return nil
}

func collectPreviewBlocks(chunk *Chunk) []blockPreview {
	dim := chunk.Dimensions()
	estimated := dim.Width * dim.Depth * dim.Height / 4
	if estimated < 16 {
		estimated = 16
	}
	blocks := make([]blockPreview, 0, estimated)
	chunk.ForEachBlock(func(coord BlockCoord, block Block) bool {
		localX, localY, localZ, ok := chunk.GlobalToLocal(coord)
		if !ok {
			return true
		}
		screenX := (localX - localY) * previewTileWidth / 2
		screenY := (localX+localY)*previewTileHeight/2 - localZ*previewBlockHeight
		blocks = append(blocks, blockPreview{
			localX:  localX,
			localY:  localY,
			localZ:  localZ,
			block:   block,
			screenX: screenX,
			screenY: screenY,
		})
		return true
	})
	return blocks
}

func renderBlockPreview(img *image.NRGBA, baseX, baseY int, block Block) {
	baseColor := resolveBlockColor(block)
	emission := clamp(block.LightEmission, 0, 1)

	topColor := applyLighting(baseColor, previewAmbientLight+0.4+0.6*emission)
	leftColor := applyLighting(baseColor, previewAmbientLight+0.25+0.4*emission)
	rightColor := applyLighting(baseColor, previewAmbientLight+0.15+0.3*emission)

	top := []image.Point{
		{X: baseX, Y: baseY - previewBlockHeight},
		{X: baseX + previewTileWidth/2, Y: baseY - previewBlockHeight + previewTileHeight/2},
		{X: baseX, Y: baseY - previewBlockHeight + previewTileHeight},
		{X: baseX - previewTileWidth/2, Y: baseY - previewBlockHeight + previewTileHeight/2},
	}
	left := []image.Point{
		{X: baseX - previewTileWidth/2, Y: baseY - previewBlockHeight + previewTileHeight/2},
		{X: baseX, Y: baseY - previewBlockHeight + previewTileHeight},
		{X: baseX, Y: baseY + previewTileHeight},
		{X: baseX - previewTileWidth/2, Y: baseY + previewTileHeight/2},
	}
	right := []image.Point{
		{X: baseX + previewTileWidth/2, Y: baseY - previewBlockHeight + previewTileHeight/2},
		{X: baseX, Y: baseY - previewBlockHeight + previewTileHeight},
		{X: baseX, Y: baseY + previewTileHeight},
		{X: baseX + previewTileWidth/2, Y: baseY + previewTileHeight/2},
	}

	fillPolygon(img, left, leftColor)
	fillPolygon(img, right, rightColor)
	fillPolygon(img, top, topColor)
}

func resolveBlockColor(block Block) color.NRGBA {
	if block.Color != "" {
		if col, ok := parseHexColor(block.Color); ok {
			return col
		}
	}
	if block.Material != "" {
		if appearance, ok := DefaultAppearances[block.Material]; ok {
			if col, ok := parseHexColor(appearance.Color); ok {
				return col
			}
		}
	}
	return color.NRGBA{R: 128, G: 128, B: 128, A: 255}
}

func parseHexColor(value string) (color.NRGBA, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return color.NRGBA{}, false
	}
	trimmed = strings.TrimPrefix(trimmed, "#")
	if len(trimmed) != 6 {
		return color.NRGBA{}, false
	}
	r, ok := parseHexByte(trimmed[0:2])
	if !ok {
		return color.NRGBA{}, false
	}
	g, ok := parseHexByte(trimmed[2:4])
	if !ok {
		return color.NRGBA{}, false
	}
	b, ok := parseHexByte(trimmed[4:6])
	if !ok {
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: r, G: g, B: b, A: 255}, true
}

func parseHexByte(value string) (uint8, bool) {
	if len(value) != 2 {
		return 0, false
	}
	v, err := strconv.ParseUint(value, 16, 8)
	if err != nil {
		return 0, false
	}
	return uint8(v), true
}

func applyLighting(base color.NRGBA, factor float64) color.NRGBA {
	factor = clamp(factor, 0, 1)
	r := uint8(math.Round(float64(base.R) * factor))
	g := uint8(math.Round(float64(base.G) * factor))
	b := uint8(math.Round(float64(base.B) * factor))
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func fillPolygon(img *image.NRGBA, pts []image.Point, col color.NRGBA) {
	if len(pts) < 3 {
		return
	}
	minY := pts[0].Y
	maxY := pts[0].Y
	for _, p := range pts[1:] {
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	bounds := img.Bounds()
	if minY < bounds.Min.Y {
		minY = bounds.Min.Y
	}
	if maxY > bounds.Max.Y-1 {
		maxY = bounds.Max.Y - 1
	}
	tmp := make([]int, 0, len(pts))
	for y := minY; y <= maxY; y++ {
		tmp = tmp[:0]
		for i := range pts {
			j := (i + 1) % len(pts)
			x1, y1 := pts[i].X, pts[i].Y
			x2, y2 := pts[j].X, pts[j].Y
			if y1 == y2 {
				continue
			}
			if y < min(y1, y2) || y >= max(y1, y2) {
				continue
			}
			x := x1 + (y-y1)*(x2-x1)/(y2-y1)
			tmp = append(tmp, x)
		}
		if len(tmp) < 2 {
			continue
		}
		sort.Ints(tmp)
		for i := 0; i+1 < len(tmp); i += 2 {
			xStart := tmp[i]
			xEnd := tmp[i+1]
			if xStart > xEnd {
				xStart, xEnd = xEnd, xStart
			}
			if xEnd < bounds.Min.X || xStart >= bounds.Max.X {
				continue
			}
			if xStart < bounds.Min.X {
				xStart = bounds.Min.X
			}
			if xEnd > bounds.Max.X-1 {
				xEnd = bounds.Max.X - 1
			}
			for x := xStart; x <= xEnd; x++ {
				idx := (y-bounds.Min.Y)*img.Stride + (x-bounds.Min.X)*4
				img.Pix[idx] = col.R
				img.Pix[idx+1] = col.G
				img.Pix[idx+2] = col.B
				img.Pix[idx+3] = col.A
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func ensurePreviewDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("output directory is empty")
	}
	return os.MkdirAll(dir, 0o755)
}
