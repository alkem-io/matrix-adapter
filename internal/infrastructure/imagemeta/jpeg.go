package imagemeta

import (
	"bytes"
	"image"
	"image/jpeg"
)

const (
	markerTEM   = 0x01
	markerRST0  = 0xD0
	markerRST7  = 0xD7
	markerSOI   = 0xD8
	markerEOI   = 0xD9
	markerSOS   = 0xDA
	markerAPP0  = 0xE0
	markerAPP1  = 0xE1
	markerAPP2  = 0xE2
	markerAPP14 = 0xEE
	markerAPP15 = 0xEF
	markerCOM   = 0xFE

	// uprightQuality is the encoder quality used when a rotated JPEG has to
	// be re-encoded.
	uprightQuality = 92
	// maxUprightPixels bounds the decode that physical rotation needs. Above
	// it the image keeps an orientation-only Exif segment instead, so a large
	// or hostile image cannot exhaust the service's memory.
	maxUprightPixels = 24_000_000
)

var (
	jfifHeader  = []byte("JFIF\x00")
	iccHeader   = []byte("ICC_PROFILE\x00")
	adobeHeader = []byte("Adobe")
	// orientationOnlyAPP1 is the APP1 marker and length (2+6+26 bytes) of an
	// Exif segment carrying orientationOnlyTIFF.
	orientationOnlyAPP1 = []byte{0xFF, markerAPP1, 0x00, 0x22}
)

// jpegScan is the result of walking a JPEG's marker structure.
type jpegScan struct {
	// out is the stream with every metadata segment removed.
	out []byte
	// exifAt is the offset in out where the first Exif segment stood, or -1.
	exifAt      int
	orientation byte
	// profile holds the ICC_PROFILE segments, which a re-encode carries over.
	profile [][]byte
	sawScan bool
}

// stripJPEG keeps the segments needed to render (tables, frame, scans, JFIF
// header without its thumbnail, ICC profile, Adobe colour transform) and drops
// every other application segment and comment: Exif (location, device, time,
// thumbnail), XMP, IPTC/Photoshop, multi-picture data, and anything appended
// after the end-of-image marker.
func stripJPEG(data []byte) ([]byte, error) {
	s, err := scanJPEG(data)
	if err != nil {
		return nil, err
	}
	if s.orientation == upright {
		return s.out, nil
	}
	if rotated, ok := s.upright(); ok {
		return rotated, nil
	}
	app1 := append(append(append([]byte{}, orientationOnlyAPP1...), exifHeader...), orientationOnlyTIFF(s.orientation)...)
	return splice(s.out, s.exifAt, app1), nil
}

func scanJPEG(data []byte) (*jpegScan, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != markerSOI {
		return nil, ErrMalformed
	}
	s := &jpegScan{out: make([]byte, 0, len(data)), exifAt: -1, orientation: upright}
	s.out = append(s.out, 0xFF, markerSOI)
	for pos := 2; ; {
		at, marker, err := nextMarker(data, pos)
		if err != nil {
			return nil, err
		}
		if marker == markerEOI {
			if !s.sawScan {
				return nil, ErrMalformed
			}
			s.out = append(s.out, 0xFF, markerEOI)
			return s, nil
		}
		if pos, err = s.consume(data, at, marker); err != nil {
			return nil, err
		}
	}
}

// consume copies or drops the marker at offset at, with its segment and any
// scan data that follows, and returns the offset of the next marker.
func (s *jpegScan) consume(data []byte, at int, marker byte) (int, error) {
	switch {
	case marker == markerSOI:
		return 0, ErrMalformed
	case marker == markerTEM || (marker >= markerRST0 && marker <= markerRST7):
		s.out = append(s.out, 0xFF, marker)
		return at + 2, nil
	}
	if at+4 > len(data) {
		return 0, ErrMalformed
	}
	end := at + 2 + (int(data[at+2])<<8 | int(data[at+3]))
	if end < at+4 || end > len(data) {
		return 0, ErrMalformed
	}
	s.keep(marker, data[at:end])
	if marker != markerSOS {
		return end, nil
	}
	s.sawScan = true
	return s.copyEntropyData(data, end)
}

// nextMarker expects a marker at pos, skipping fill bytes, and returns the
// offset of the 0xFF that immediately precedes the marker code.
func nextMarker(data []byte, pos int) (int, byte, error) {
	if pos >= len(data) || data[pos] != 0xFF {
		return 0, 0, ErrMalformed
	}
	for pos+1 < len(data) && data[pos+1] == 0xFF {
		pos++
	}
	if pos+1 >= len(data) || data[pos+1] == 0x00 {
		return 0, 0, ErrMalformed
	}
	return pos, data[pos+1], nil
}

// copyEntropyData copies the entropy-coded data after a scan header up to the
// next marker that is not a stuffed byte or a restart marker, and returns that
// marker's offset. Running out of data means the stream is truncated.
func (s *jpegScan) copyEntropyData(data []byte, pos int) (int, error) {
	i := pos
	for {
		next := bytes.IndexByte(data[i:], 0xFF)
		if next < 0 || i+next+1 >= len(data) {
			return 0, ErrMalformed
		}
		i += next
		switch code := data[i+1]; {
		case code == 0x00, code >= markerRST0 && code <= markerRST7:
			i += 2
		case code == 0xFF:
			i++
		default:
			s.out = append(s.out, data[pos:i]...)
			return i, nil
		}
	}
}

func (s *jpegScan) keep(marker byte, segment []byte) {
	payload := segment[4:]
	switch {
	case marker == markerAPP1 && bytes.HasPrefix(payload, exifHeader):
		if s.exifAt < 0 {
			s.exifAt = len(s.out)
			s.orientation = orientation(payload)
		}
	case marker == markerAPP0:
		s.out = append(s.out, jfifWithoutThumbnail(payload)...)
	case marker == markerAPP2 && bytes.HasPrefix(payload, iccHeader):
		s.out = append(s.out, segment...)
		s.profile = append(s.profile, segment)
	case marker == markerAPP14 && bytes.HasPrefix(payload, adobeHeader):
		s.out = append(s.out, segment...)
	case marker >= markerAPP0 && marker <= markerAPP15, marker == markerCOM:
		// Every other application segment and every comment is metadata.
	default:
		s.out = append(s.out, segment...)
	}
}

// jfifWithoutThumbnail rewrites a JFIF APP0 payload with its embedded
// thumbnail removed, since a thumbnail can show what a crop has hidden. Any
// other APP0 (such as a JFXX thumbnail extension) is dropped.
func jfifWithoutThumbnail(payload []byte) []byte {
	const fields = 14 // identifier, version, density units, x/y density, thumbnail size
	if len(payload) < fields || !bytes.HasPrefix(payload, jfifHeader) {
		return nil
	}
	segment := append([]byte{0xFF, markerAPP0, 0x00, 2 + fields}, payload[:fields]...)
	segment[len(segment)-2], segment[len(segment)-1] = 0, 0
	return segment
}

// upright decodes, rotates and re-encodes the stripped stream so the pixels
// themselves face the right way. ok is false when the image is too large to
// decode safely or uses a coding the standard decoder does not support.
func (s *jpegScan) upright() ([]byte, bool) {
	config, err := jpeg.DecodeConfig(bytes.NewReader(s.out))
	if err != nil || int64(config.Width)*int64(config.Height) > maxUprightPixels {
		return nil, false
	}
	img, err := jpeg.Decode(bytes.NewReader(s.out))
	if err != nil {
		return nil, false
	}
	rotated := orient(img, s.orientation)
	var encoded bytes.Buffer
	encoded.Grow(len(s.out))
	if err := jpeg.Encode(&encoded, rotated, &jpeg.Options{Quality: uprightQuality}); err != nil {
		return nil, false
	}
	body := encoded.Bytes()
	out := make([]byte, 0, len(body)+profileSize(s.profile))
	out = append(out, body[:2]...)
	// A profile describes YCbCr or grey samples; a CMYK source is converted
	// to RGB on the way through, so its CMYK profile would no longer apply.
	switch img.(type) {
	case *image.YCbCr, *image.Gray:
		for _, segment := range s.profile {
			out = append(out, segment...)
		}
	}
	return append(out, body[2:]...), true
}

func profileSize(segments [][]byte) int {
	size := 0
	for _, segment := range segments {
		size += len(segment)
	}
	return size
}
