package tools

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	_ "image/gif" // register GIF decoder (stdlib, no cgo)
	"image/jpeg"
	"image/png"
	"math"
)

// Image size limits for vision payloads.
//
// A single base64 image block that exceeds maxImageBase64Bytes, or whose
// longest edge exceeds maxImageDimension, is downscaled before it ever reaches
// the provider. This is the proactive half of the fix for HTTP 413
// "payload_too_large": most gateways cap the request body around 4-6 MB, and a
// single Retina screenshot (PNG, 3-5 MB → ~5-7 MB base64) blows past that on
// its own. Shrinking the image is far better UX than dropping it, and keeps
// the picture usable for vision models (they typically down-sample to ~1.5 MP
// internally anyway).
const (
	// maxImageBase64Bytes — if the base64 payload is larger than this we
	// re-encode the image at lower quality/smaller dimensions. ~3 MB base64
	// keeps the whole request comfortably under a 4-6 MB gateway limit even
	// with several images plus text in the same turn.
	maxImageBase64Bytes = 3 * 1024 * 1024

	// maxImageDimension — longest edge (px) after downscale. ~1568 px is the
	// size Anthropic's vision encoder resamples to, so we lose no model-side
	// detail by capping here while cutting pixel count ~4x on Retina shots.
	maxImageDimension = 1568

	// jpegReencodeQuality — quality used when we must shrink a JPEG that is
	// already within the dimension cap but still too heavy in bytes.
	jpegReencodeQuality = 80
)

// DownscaleImageBase64 takes base64-encoded image data plus its format and
// returns a (possibly) smaller base64 payload that stays within the size
// limits. It is a best-effort operation: if the image is already small, or its
// format cannot be decoded with the standard library (e.g. WEBP/BMP/TIFF), the
// original data is returned unchanged so callers never lose content due to a
// resize failure. The returned bool reports whether the payload was modified.
//
// Supported for resize: PNG, JPEG, GIF (all decodable via the standard
// library, no cgo, no third-party deps).
func DownscaleImageBase64(base64Data, format string) (string, string, bool) {
	return DownscaleImageBase64To(base64Data, format, maxImageBase64Bytes)
}

// DownscaleImageBase64To is DownscaleImageBase64 with an explicit base64 byte
// budget. The agent's 413 self-heal calls it with a progressively smaller
// budget to shrink history images that still overflow the gateway limit.
func DownscaleImageBase64To(base64Data, format string, maxBytes int) (string, string, bool) {
	raw, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return base64Data, format, false
	}

	// Fast path: already small enough in bytes AND we don't know dimensions —
	// but a small-bytes PNG/JPEG can still be huge in pixels. Decode to check.
	img, decFmt, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		// Format not decodable by stdlib (webp/bmp/tiff/...) — leave as-is.
		return base64Data, format, false
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Dimension cap tightens with the byte budget: the default budget keeps
	// maxImageDimension; a smaller budget (413 self-heal) scales down harder so
	// even a photo-heavy history can be squeezed under the gateway limit.
	dimCap := maxImageDimension
	if maxBytes < maxImageBase64Bytes {
		dimCap = maxImageDimension * maxBytes / maxImageBase64Bytes
		if dimCap < 384 {
			dimCap = 384
		}
	}

	needsResize := w > dimCap || h > dimCap
	tooHeavy := len(base64Data) > maxBytes

	if !needsResize && !tooHeavy {
		return base64Data, format, false
	}

	// Compute target dimensions. If only too heavy (not oversized in px), keep
	// dimensions but re-encode with quality; if oversized, scale down.
	outImg := img
	outFmt := decFmt
	if needsResize {
		scale := math.Min(float64(dimCap)/float64(w), float64(dimCap)/float64(h))
		nw := int(math.Round(float64(w) * scale))
		nh := int(math.Round(float64(h) * scale))
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
		outImg = boxResize(img, nw, nh)
	}

	// Encode. Prefer JPEG for opaque images (much smaller); keep PNG when the
	// source had alpha so we don't drop transparency.
	hasAlpha := imageHasAlpha(img)
	var buf bytes.Buffer
	switch {
	case outFmt == "jpeg" || (!hasAlpha && !needsAlphaPreserve(outFmt)):
		// JPEG output.
		if err := jpeg.Encode(&buf, toOpaque(outImg), &jpeg.Options{Quality: jpegReencodeQuality}); err != nil {
			return base64Data, format, false
		}
		outFmt = "jpeg"
	default:
		// PNG output (preserves alpha / lossless for screenshots with text).
		if err := png.Encode(&buf, outImg); err != nil {
			return base64Data, format, false
		}
		outFmt = "png"
	}

	newB64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	// Only accept the re-encode if it actually helped; otherwise keep original
	// (e.g. a tiny PNG that grew when converted to JPEG).
	if len(newB64) >= len(base64Data) && !needsResize {
		return base64Data, format, false
	}

	// Normalise the format string the caller uses (jpeg vs jpg).
	outFormat := normalizeImgFormat(outFmt)
	return newB64, outFormat, true
}

// needsAlphaPreserve reports whether a format must keep an alpha channel and
// therefore cannot be flattened to JPEG.
func needsAlphaPreserve(fmtName string) bool {
	switch fmtName {
	case "png", "gif":
		return true
	default:
		return false
	}
}

// normalizeImgFormat maps decoder format names to the strings used elsewhere
// in the codebase (Anthropic/OpenAI media types).
func normalizeImgFormat(fmtName string) string {
	if fmtName == "jpg" {
		return "jpeg"
	}
	return fmtName
}

// imageHasAlpha reports whether the image actually uses a non-opaque alpha
// channel (PNG/GIF with transparency).
func imageHasAlpha(img image.Image) bool {
	b := img.Bounds()
	// Sample a sparse grid so we don't scan every pixel on huge images.
	step := 1
	if w := b.Dx(); w > 256 {
		step = w / 256
	}
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xffff {
				return true
			}
		}
	}
	return false
}

// toOpaque returns an image with the alpha channel flattened onto white, so it
// can be encoded as JPEG (which has no alpha). If the image is already opaque
// NRGBA/RGBA it is returned unchanged.
func toOpaque(img image.Image) image.Image {
	if _, ok := img.(*image.YCbCr); ok {
		return img
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	// Fill white background.
	white := color.RGBA{255, 255, 255, 255}
	for y := 0; y < out.Bounds().Dy(); y++ {
		for x := 0; x < out.Bounds().Dx(); x++ {
			out.SetRGBA(x, y, white)
		}
	}
	// Composite source over white.
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a == 0 {
				continue
			}
			// At().RGBA() returns premultiplied 16-bit values. Compositing a
			// premultiplied source over an opaque background is just
			//   result = src + dst*(1 - srcAlpha)
			// with src already scaled by its own alpha.
			af := float64(a) / 65535.0
			nr := uint8(math.Min(255.0, float64(r>>8)+float64(255)*(1.0-af)))
			ng := uint8(math.Min(255.0, float64(g>>8)+float64(255)*(1.0-af)))
			nb := uint8(math.Min(255.0, float64(bl>>8)+float64(255)*(1.0-af)))
			out.SetRGBA(x, y, color.RGBA{nr, ng, nb, 255})
		}
	}
	return out
}

// boxResize downsamples src to dst dimensions using a box (area-average)
// filter, which is the correct choice for shrinking and avoids the aliasing a
// naive nearest-neighbour produces on Retina screenshots. Implemented with the
// standard library only (no cgo, no third-party deps).
func boxResize(src image.Image, dstW, dstH int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	// Pre-read source into an RGBA slice for fast, cache-friendly access.
	srcRGBA := image.NewRGBA(image.Rect(0, 0, sw, sh))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			srcRGBA.SetRGBA(x, y, nrgbaToRGBA(color.NRGBAModel.Convert(src.At(sb.Min.X+x, sb.Min.Y+y)).(color.NRGBA)))
		}
	}

	// For each destination pixel, average the source pixels that map into it.
	// dst pixel (dx,dy) covers source rows [sy0,sy1) and cols [sx0,sx1).
	for dy := 0; dy < dstH; dy++ {
		sy0 := dy * sh / dstH
		sy1 := (dy + 1) * sh / dstH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		if sy1 > sh {
			sy1 = sh
		}
		for dx := 0; dx < dstW; dx++ {
			sx0 := dx * sw / dstW
			sx1 := (dx + 1) * sw / dstW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sx1 > sw {
				sx1 = sw
			}
			var sumR, sumG, sumB, sumA, count uint64
			for sy := sy0; sy < sy1; sy++ {
				rowOff := sy * srcRGBA.Stride
				for sx := sx0; sx < sx1; sx++ {
					i := rowOff + sx*4
					sumA += uint64(srcRGBA.Pix[i+3])
					// Premultiply for a correct average with alpha.
					a := srcRGBA.Pix[i+3]
					sumR += uint64(srcRGBA.Pix[i]) * uint64(a)
					sumG += uint64(srcRGBA.Pix[i+1]) * uint64(a)
					sumB += uint64(srcRGBA.Pix[i+2]) * uint64(a)
					count++
				}
			}
			if count == 0 {
				continue
			}
			var r, g, b, al uint8
			if sumA == 0 {
				// Fully transparent block.
				dst.SetRGBA(dx, dy, color.RGBA{0, 0, 0, 0})
				continue
			}
			// Average premultiplied values, then un-premultiply.
			avgA := sumA / count
			r = uint8(minU(sumR/count*255/avgA, 255))
			g = uint8(minU(sumG/count*255/avgA, 255))
			b = uint8(minU(sumB/count*255/avgA, 255))
			al = uint8(minU(avgA, 255))
			dst.SetRGBA(dx, dy, color.RGBA{r, g, b, al})
		}
	}
	return dst
}

// minU clamps a uint64 to a uint8 ceiling.
func minU(v, max uint64) uint64 {
	if v > max {
		return max
	}
	return v
}

// nrgbaToRGBA converts a non-premultiplied NRGBA to premultiplied RGBA, for
// fast cache-friendly box sampling.
func nrgbaToRGBA(c color.NRGBA) color.RGBA {
	a := uint32(c.A)
	return color.RGBA{
		R: uint8(uint32(c.R) * a / 255),
		G: uint8(uint32(c.G) * a / 255),
		B: uint8(uint32(c.B) * a / 255),
		A: c.A,
	}
}
