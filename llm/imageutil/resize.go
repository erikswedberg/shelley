// Package imageutil provides image manipulation utilities.
package imageutil

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"

	"golang.org/x/image/draw"
)

// DecodeDimensions returns the pixel width and height of the image encoded in
// data. It reads only the image header (no full decode), so it is cheap.
func DecodeDimensions(data []byte) (width, height int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("decode image config: %w", err)
	}
	return cfg.Width, cfg.Height, nil
}

// Validate fully decodes data to confirm it is a complete, well-formed image.
// Unlike DecodeDimensions (header-only) and http.DetectContentType (magic-byte
// sniff), this walks every pixel chunk, so it catches truncated or otherwise
// corrupt files whose header still looks valid. A partial upload over a flaky
// link is the motivating case: its PNG header advertises the right dimensions
// but the pixel data is cut short. Embedding such bytes in a message makes the
// provider reject the whole request (400 "could not process image"), which
// wedges the conversation permanently. Validating here turns that into a
// recoverable tool error instead.
//
// Formats without a decoder registered in this binary (currently anything but
// PNG and JPEG — e.g. WebP or GIF) surface as image.ErrFormat; we cannot
// verify those here, so we let them through rather than reject a valid image
// we simply can't decode. Only a genuine decode failure of a recognized format
// (the truncation case) is reported as an error.
func Validate(data []byte) error {
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		if errors.Is(err, image.ErrFormat) {
			return nil
		}
		return fmt.Errorf("decode image: %w", err)
	}
	return nil
}

// ResizeImage resizes an image if any dimension exceeds maxDimension.
// Returns the resized image bytes and the format ("png" or "jpeg").
// If no resize is needed, returns the original data unchanged.
func ResizeImage(data []byte, maxDimension int) (resized []byte, format string, didResize bool, err error) {
	img, detectedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to decode image: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width <= maxDimension && height <= maxDimension {
		return data, detectedFormat, false, nil
	}

	// Calculate new dimensions preserving aspect ratio
	newWidth, newHeight := width, height
	if width > height {
		newWidth = maxDimension
		newHeight = height * maxDimension / width
	} else {
		newHeight = maxDimension
		newWidth = width * maxDimension / height
	}

	// Create resized image
	resizedImg := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.BiLinear.Scale(resizedImg, resizedImg.Bounds(), img, bounds, draw.Over, nil)

	// Encode to the same format
	var buf bytes.Buffer
	switch strings.ToLower(detectedFormat) {
	case "jpeg", "jpg":
		err = jpeg.Encode(&buf, resizedImg, &jpeg.Options{Quality: 85})
		format = "jpeg"
	default:
		err = png.Encode(&buf, resizedImg)
		format = "png"
	}

	if err != nil {
		return nil, "", false, fmt.Errorf("failed to encode resized image: %w", err)
	}

	return buf.Bytes(), format, true, nil
}

const (
	// MaxBase64Size is the max base64-encoded image size the Anthropic API accepts (5MB).
	MaxBase64Size = 5 * 1024 * 1024
	// TargetRawSize is the target raw byte size (3/4 of base64 limit).
	TargetRawSize = MaxBase64Size * 3 / 4
)

// EnsureUnderMaxBytes compresses and/or resizes an image to stay under the
// API's base64 size limit. Returns the (possibly modified) image data, the
// media type, and any error. Mirrors the Claude CLI cascade.
func EnsureUnderMaxBytes(data []byte) ([]byte, string, error) {
	if len(data) <= TargetRawSize {
		// Small enough — detect format and return as-is.
		_, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			format = "png"
		}
		return data, "image/" + format, nil
	}

	// Decode the image for re-encoding.
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image: %w", err)
	}

	// Try JPEG at decreasing quality.
	for _, q := range []int{80, 60, 40, 20} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			continue
		}
		if buf.Len() <= TargetRawSize {
			return buf.Bytes(), "image/jpeg", nil
		}
	}

	// Resize to smaller dimensions + JPEG q60.
	bounds := img.Bounds()
	for _, scale := range []float64{0.75, 0.50, 0.25} {
		nw := int(float64(bounds.Dx()) * scale)
		nh := int(float64(bounds.Dy()) * scale)
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
		resized := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.BiLinear.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 60}); err != nil {
			continue
		}
		if buf.Len() <= TargetRawSize {
			return buf.Bytes(), "image/jpeg", nil
		}
	}

	// Last resort: 400×400 (preserving aspect ratio) at JPEG q20.
	maxDim := 400
	nw, nh := maxDim, maxDim
	if bounds.Dx() > bounds.Dy() {
		nh = bounds.Dy() * maxDim / bounds.Dx()
	} else {
		nw = bounds.Dx() * maxDim / bounds.Dy()
	}
	resized := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.BiLinear.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 20}); err != nil {
		return nil, "", fmt.Errorf("failed to encode last-resort image: %w", err)
	}
	return buf.Bytes(), "image/jpeg", nil
}
