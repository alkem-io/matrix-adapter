// Package imagemeta removes privacy-relevant metadata (location, device
// identifiers, timestamps, comments, embedded thumbnails) from uploaded images
// before they are stored, keeping what is needed to render them faithfully:
// pixel data, colour profiles, transparency and animation.
//
// The container is identified from its leading bytes, never from a declared
// content type. JPEG, PNG and WebP are rewritten; every other format is left
// to the caller to pass through unchanged.
package imagemeta

import (
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

type signature struct {
	format Format
	magic  []byte
	// wildFrom/wildTo bound a half-open range of bytes that may hold anything.
	wildFrom, wildTo int
}

var signatures = []signature{
	{format: FormatJPEG, magic: []byte{0xFF, 0xD8, 0xFF}},
	{format: FormatPNG, magic: pngSignature},
	{format: FormatWebP, magic: []byte("RIFF\x00\x00\x00\x00WEBP"), wildFrom: 4, wildTo: 8},
}

func (s signature) prefixOf(head []byte) bool {
	for i := 0; i < len(head) && i < len(s.magic); i++ {
		if (i < s.wildFrom || i >= s.wildTo) && head[i] != s.magic[i] {
			return false
		}
	}
	return true
}

// Sniff identifies the container from the leading bytes read so far. decided
// is false while head is still too short to tell, so a caller sniffing a
// stream can stop reading as soon as the answer is known.
func Sniff(head []byte) (format Format, decided bool) {
	undecided := false
	for _, s := range signatures {
		if !s.prefixOf(head) {
			continue
		}
		if len(head) >= len(s.magic) {
			return s.format, true
		}
		undecided = true
	}
	return FormatNone, !undecided
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

// Strip returns data without privacy-relevant metadata. A JPEG whose Exif
// orientation is not upright is physically rotated and re-encoded so it still
// displays upright; every other image is rewritten losslessly by removing
// metadata segments or chunks. The result is a new slice; data is not changed.
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

// splice returns b with insert placed at offset at.
func splice(b []byte, at int, insert []byte) []byte {
	out := make([]byte, 0, len(b)+len(insert))
	out = append(out, b[:at]...)
	out = append(out, insert...)
	return append(out, b[at:]...)
}
