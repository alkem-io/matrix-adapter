package imagemeta

import "encoding/binary"

const (
	webpFlagEXIF = 0x08
	webpFlagXMP  = 0x04

	chunkVP8X = "VP8X"
	chunkEXIF = "EXIF"
	chunkVP8  = "VP8\x20" // the lossy bitstream; its identifier ends in a space
	chunkVP8L = "VP8L"
	chunkANMF = "ANMF"
)

// webpKept lists the chunks needed to render a WebP: the extended header,
// colour profile, animation, alpha and image bitstreams. EXIF, XMP and any
// unknown chunk are dropped.
var webpKept = map[string]bool{
	chunkVP8X: true, "ICCP": true, "ANIM": true, chunkANMF: true,
	"ALPH": true, chunkVP8: true, chunkVP8L: true,
}

// webpScan is the state of a WebP rewrite.
type webpScan struct {
	out []byte
	// header is the offset in out of the VP8X chunk, or -1.
	header      int
	orientation byte
	sawImage    bool
	sawExif     bool
}

// stripWebP rewrites a WebP losslessly, keeping only the chunks in webpKept,
// clearing the EXIF and XMP flags in the extended header and recomputing the
// RIFF size. An Exif orientation other than upright is kept as an
// orientation-only EXIF chunk, and the image displays exactly as it did before
// wherever that was honoured. The output is sized for that chunk up front, so
// it never reallocates.
func stripWebP(data []byte) ([]byte, error) {
	if len(data) < 12 {
		return nil, ErrMalformed
	}
	end := 8 + uint64(binary.LittleEndian.Uint32(data[4:]))
	if end < 12 || end > uint64(len(data)) {
		return nil, ErrMalformed
	}
	s := &webpScan{out: append(make([]byte, 0, end+8+orientationOnlyLen), data[:12]...), header: -1, orientation: upright}
	for pos := uint64(12); pos < end; {
		next, err := s.chunk(data, pos, end)
		if err != nil {
			return nil, err
		}
		pos = next
	}
	if !s.sawImage {
		return nil, ErrMalformed
	}
	s.finishHeader()
	binary.LittleEndian.PutUint32(s.out[4:], uint32(len(s.out)-8)) //nolint:gosec // out is the copied RIFF minus dropped chunks, plus at most 34 bytes
	return s.out, nil
}

// chunk copies or drops the chunk at pos and returns the offset of the next.
func (s *webpScan) chunk(data []byte, pos, end uint64) (uint64, error) {
	if pos+8 > end {
		return 0, ErrMalformed
	}
	kind := string(data[pos : pos+4])
	size := uint64(binary.LittleEndian.Uint32(data[pos+4:]))
	next := pos + 8 + size + size&1
	if next > end {
		return 0, ErrMalformed
	}
	switch {
	case kind == chunkEXIF && !s.sawExif:
		s.sawExif, s.orientation = true, orientation(data[pos+8:pos+8+size])
	case kind == chunkVP8X:
		if size < 10 || s.header >= 0 {
			return 0, ErrMalformed
		}
		s.header = len(s.out)
		s.out = append(s.out, data[pos:next]...)
	case webpKept[kind]:
		s.out = append(s.out, data[pos:next]...)
	}
	s.sawImage = s.sawImage || kind == chunkVP8 || kind == chunkVP8L || kind == chunkANMF
	return next, nil
}

// finishHeader clears the metadata flags of the extended header and, for an
// image that is not upright, appends the orientation-only EXIF chunk. A simple
// (VP8X-less) WebP cannot declare EXIF, so it gets none.
func (s *webpScan) finishHeader() {
	if s.header < 0 {
		return
	}
	flags := s.out[s.header+8] &^ (webpFlagEXIF | webpFlagXMP)
	if s.orientation != upright {
		payload := orientationOnlyTIFF(s.orientation)
		s.out = append(s.out, chunkEXIF...)
		s.out = binary.LittleEndian.AppendUint32(s.out, uint32(len(payload))) //nolint:gosec // payload is a fixed 26-byte structure
		s.out = append(s.out, payload...)
		flags |= webpFlagEXIF
	}
	s.out[s.header+8] = flags
}
