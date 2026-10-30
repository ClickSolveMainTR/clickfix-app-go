package usecase

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
)

const (
	// thumbMaxEdge is the longest edge of a generated thumbnail in pixels.
	thumbMaxEdge = 320
	// thumbMaxSourcePixels guards against decompression bombs: larger
	// images are stored without a thumbnail instead of being decoded.
	thumbMaxSourcePixels = 40_000_000
	thumbJPEGQuality     = 80
)

// buildThumbnail returns a JPEG thumbnail of a raster image, or nil when the
// type is not supported (webp, svg, ...), the image is too large to decode
// safely, or decoding fails. A missing thumbnail never fails an upload.
func buildThumbnail(contentType string, data []byte) []byte {
	switch contentType {
	case "image/jpeg", "image/png", "image/gif":
	default:
		return nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil
	}
	if int64(cfg.Width)*int64(cfg.Height) > thumbMaxSourcePixels {
		return nil
	}
	var src image.Image
	switch contentType {
	case "image/jpeg":
		src, err = jpeg.Decode(bytes.NewReader(data))
	case "image/png":
		src, err = png.Decode(bytes.NewReader(data))
	case "image/gif":
		src, err = gif.Decode(bytes.NewReader(data))
	}
	if err != nil || src == nil {
		return nil
	}
	dst := scaleToFit(src, thumbMaxEdge)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: thumbJPEGQuality}); err != nil {
		return nil
	}
	return out.Bytes()
}

// scaleToFit box-samples src down so its longest edge is at most maxEdge. The
// result is flattened on white so transparent PNG/GIF pixels do not turn black
// in JPEG. Images already small enough are only flattened.
func scaleToFit(src image.Image, maxEdge int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w, h
	if w > maxEdge || h > maxEdge {
		if w >= h {
			nw = maxEdge
			nh = max(1, h*maxEdge/w)
		} else {
			nh = maxEdge
			nw = max(1, w*maxEdge/h)
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for y := 0; y < nh; y++ {
		sy0 := b.Min.Y + y*h/nh
		sy1 := max(sy0+1, b.Min.Y+(y+1)*h/nh)
		for x := 0; x < nw; x++ {
			sx0 := b.Min.X + x*w/nw
			sx1 := max(sx0+1, b.Min.X+(x+1)*w/nw)
			var r, g, bl, a, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					pr, pg, pb, pa := src.At(sx, sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					bl += uint64(pb)
					a += uint64(pa)
					n++
				}
			}
			if n == 0 {
				continue
			}
			// Premultiplied average composited over white.
			ar, ag, ab, aa := r/n, g/n, bl/n, a/n
			inv := 0xffff - aa
			dst.SetRGBA(x, y, color.RGBA{
				R: safeconv.ToUint8((ar + inv) >> 8),
				G: safeconv.ToUint8((ag + inv) >> 8),
				B: safeconv.ToUint8((ab + inv) >> 8),
				A: 0xff,
			})
		}
	}
	return dst
}
