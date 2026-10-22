// Package imagemeta removes location, device, time and other privacy-relevant
// metadata from JPEG, PNG and WebP images, identified from their bytes, keeping
// pixels, colour profiles, transparency and animation.
package imagemeta

import (
	"bytes"
	"errors"
	"fmt"
)

// ErrMalformed reports a recognised image container whose structure is broken.
// Callers must refuse the upload rather than store the unstripped bytes.
var ErrMalformed = errors.New("malformed image")

// Format identifies an image container this package rewrites.
type Format int

const (
	// FormatNone is any content this package does not rewrite.
	FormatNone Format = iota
	// FormatJPEG is a JPEG (JFIF/Exif) stream.
	FormatJPEG
	// FormatPNG is a PNG (or animated PNG) datastream.
	FormatPNG
	// FormatWebP is a RIFF WebP file, simple or extended.
	FormatWebP
)

// SniffLen is the number of leading bytes Sniff needs for a definite answer.
const SniffLen = 12

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// Sniff identifies the container from its leading bytes.
func Sniff(head []byte) Format {
	switch {
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return FormatJPEG
	case bytes.HasPrefix(head, pngSignature):
		return FormatPNG
	case len(head) >= SniffLen && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return FormatWebP
	default:
		return FormatNone
	}
}

// ContentType is the media type of a rewritten image of this format.
func (f Format) ContentType() string {
	switch f {
	case FormatJPEG:
		return "image/jpeg"
	case FormatPNG:
		return "image/png"
	case FormatWebP:
		return "image/webp"
	default:
		return ""
	}
}

// Strip returns data without privacy-relevant metadata, rewritten losslessly
// by removing metadata segments or chunks; the pixel data is never decoded.
// An Exif orientation other than upright is kept as an orientation-only Exif
// field, so the image still displays the right way up. The result is a new
// slice, sized so that building it never copies it again; data is not changed.
func Strip(format Format, data []byte) ([]byte, error) {
	var (
		out []byte
		err error
	)
	switch format {
	case FormatJPEG:
		out, err = stripJPEG(data)
	case FormatPNG:
		out, err = stripPNG(data)
	case FormatWebP:
		out, err = stripWebP(data)
	default:
		return nil, fmt.Errorf("imagemeta: unsupported format %d", format)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", format.ContentType(), err)
	}
	return out, nil
}
