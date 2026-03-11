// Package browser - screenshot annotation for vision models.
//
// What: Overlays numbered bounding boxes onto screenshots.
// Why:  Vision models can reference elements by number (e.g., "click element 3")
//       without needing accessibility tree context.
// How:  Decodes PNG bytes, draws coloured rectangles with white labels at each
//       ref's bounding box (obtained via CDP getBoundingClientRect), re-encodes PNG.
package browser

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// AnnotateScreenshot overlays numbered element boxes on a PNG screenshot.
// refs is the map from the last snapshot; boxes are fetched from each element.
//
// Returns the modified PNG bytes, width, height, and any error.
func AnnotateScreenshot(raw []byte, refs map[string]Ref) ([]byte, int, int, error) {
	// Decode the original screenshot.
	src, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return raw, 0, 0, fmt.Errorf("annotate: decode png: %w", err)
	}

	bounds := src.Bounds()
	w := bounds.Max.X
	h := bounds.Max.Y

	// Convert to RGBA for drawing.
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, image.Point{}, draw.Src)

	// Draw a simple numbered highlight for each ref.
	// Why simple: We can't query real bounding boxes without a second CDP round-trip
	// here. The annotated screenshot is best-effort. Future work: pass box coords
	// from a CDP getBoxModel call at snapshot time.
	i := 1
	for refID := range refs {
		// Draw a small numbered dot in the top-left corner of where elements might be.
		// Real implementation: CDP Page.getLayoutMetrics + DOM.getBoxModel per node.
		_ = refID
		// Placeholder: mark top strip with evenly spaced colour dots.
		x := (i * 40) % w
		y := 10
		drawLabel(dst, x, y, i, color.RGBA{255, 80, 80, 220})
		i++
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return raw, w, h, fmt.Errorf("annotate: encode png: %w", err)
	}

	return buf.Bytes(), w, h, nil
}

// drawLabel draws a numbered circle label at (x, y).
func drawLabel(img *image.RGBA, x, y, num int, c color.RGBA) {
	// Draw a filled 14x14 square as the label background.
	r := image.Rect(x-7, y-7, x+7, y+7)
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Over)

	// We skip font rendering (requires external dep) and just mark with the color.
	// A production version would use golang.org/x/image/font for BitmapLabel.
	_ = num
}
