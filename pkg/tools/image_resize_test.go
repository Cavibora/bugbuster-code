package tools

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"testing"
)

// makeNoisePNG builds a hard-to-compress PNG (pseudo-random noise so the
// encoder can't shrink it losslessly) and returns the encoded bytes. Pixels are
// written straight into the Pix buffer (not via per-pixel Set) to keep the test
// fast even under -race. Width is chosen > maxImageDimension so the resize path
// is exercised without generating millions of pixels.
func makeNoisePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(12345)
	for i := 0; i < len(img.Pix); i += 4 {
		seed = seed*1664525 + 1013904223
		img.Pix[i] = uint8(seed >> 16)  // R
		img.Pix[i+1] = uint8(seed >> 8) // G
		img.Pix[i+2] = uint8(seed)      // B
		img.Pix[i+3] = 255              // A (opaque)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// oversizedW is just above maxImageDimension so needsResize fires while keeping
// the pixel count (and thus test runtime) small.
const oversizedW = maxImageDimension + 32

func TestDownscaleImageBase64_ShrinksLargeImage(t *testing.T) {
	raw := makeNoisePNG(t, oversizedW, 100)
	b64 := base64.StdEncoding.EncodeToString(raw)

	newB64, newFmt, changed := DownscaleImageBase64(b64, "png")
	if !changed {
		t.Fatalf("expected a %dx100 image (> cap %d) to be downscaled", oversizedW, maxImageDimension)
	}
	if len(newB64) >= len(b64) {
		t.Errorf("downscaled payload should be smaller: %d >= %d", len(newB64), len(b64))
	}
	if newFmt != "png" && newFmt != "jpeg" {
		t.Errorf("unexpected output format %q", newFmt)
	}

	// The result must still decode and respect the dimension cap.
	decoded, err := base64.StdEncoding.DecodeString(newB64)
	if err != nil {
		t.Fatalf("result not valid base64: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("result not a decodable image: %v", err)
	}
	b := img.Bounds()
	if b.Dx() > maxImageDimension || b.Dy() > maxImageDimension {
		t.Errorf("downscaled image exceeds cap: %dx%d (cap %d)", b.Dx(), b.Dy(), maxImageDimension)
	}
}

func TestDownscaleImageBase64_LeavesSmallImageAlone(t *testing.T) {
	raw := makeNoisePNG(t, 100, 80)
	b64 := base64.StdEncoding.EncodeToString(raw)

	newB64, _, changed := DownscaleImageBase64(b64, "png")
	if changed {
		t.Error("a small image must not be modified")
	}
	if newB64 != b64 {
		t.Error("unchanged image must be returned byte-for-byte")
	}
}

func TestDownscaleImageBase64_UndecodableFormatPassesThrough(t *testing.T) {
	// "webp" bytes that stdlib can't decode must be returned unchanged rather
	// than dropped (best-effort contract).
	fake := base64.StdEncoding.EncodeToString([]byte("not a real decodable image"))
	newB64, fmt, changed := DownscaleImageBase64(fake, "webp")
	if changed {
		t.Error("undecodable format must not report a change")
	}
	if newB64 != fake || fmt != "webp" {
		t.Error("undecodable payload must pass through unchanged")
	}
}

func TestDownscaleImageBase64To_HonoursTightBudget(t *testing.T) {
	raw := makeNoisePNG(t, oversizedW, 100)
	b64 := base64.StdEncoding.EncodeToString(raw)

	// A very tight budget should shrink harder than the default.
	loose, _, _ := DownscaleImageBase64(b64, "png")
	tight, _, changed := DownscaleImageBase64To(b64, "png", 200*1024)
	if !changed {
		t.Fatal("expected change under tight budget")
	}
	if len(tight) >= len(loose) {
		t.Errorf("tight budget (%d) should be smaller than default (%d)", len(tight), len(loose))
	}
}

func TestExtractImageFromResult_AppliesDownscale(t *testing.T) {
	raw := makeNoisePNG(t, oversizedW, 100)
	b64 := base64.StdEncoding.EncodeToString(raw)
	res := ToolResult{Metadata: map[string]any{
		"image_base64": b64,
		"image_format": "png",
	}}

	gotB64, gotFmt, ok := ExtractImageFromResult(res)
	if !ok {
		t.Fatal("expected image extraction to succeed")
	}
	if gotFmt == "" {
		t.Error("expected a non-empty format")
	}
	// ExtractImageFromResult must hand back a payload no larger than the input
	// (and strictly smaller here, since the source is oversized).
	if len(gotB64) > len(b64) {
		t.Errorf("extracted payload larger than source: %d > %d", len(gotB64), len(b64))
	}
}
