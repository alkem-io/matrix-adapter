package imagemeta

import "bytes"

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
)

var (
	jfifHeader  = []byte("JFIF\x00")
	iccHeader   = []byte("ICC_PROFILE\x00")
	adobeHeader = []byte("Adobe")
	// orientationOnlyAPP1 is the APP1 marker and length (2+6+26 bytes) of an
	// Exif segment carrying orientationOnlyTIFF.
	orientationOnlyAPP1 = []byte{0xFF, markerAPP1, 0x00, 0x22}
)

// jpegScan is the state of a JPEG rewrite.
type jpegScan struct {
	// out is the stream with every metadata segment removed.
	out     []byte
	sawExif bool
	sawScan bool
}

// stripJPEG keeps the segments needed to render (tables, frame, scans, JFIF
// header without its thumbnail, ICC profile, Adobe colour transform) and drops
// every other application segment and comment: Exif (location, device, time,
// thumbnail), XMP, IPTC/Photoshop, multi-picture data, and anything appended
// after the end-of-image marker. An Exif orientation other than upright is
// kept as an orientation-only Exif segment where the Exif segment stood, so
// the image still displays the right way up; the pixels are never decoded.
func stripJPEG(data []byte) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != markerSOI {
		return nil, ErrMalformed
	}
	// Room for the orientation-only segment, so the output never reallocates.
	s := &jpegScan{out: make([]byte, 0, len(data)+len(orientationOnlyAPP1)+len(exifHeader)+orientationOnlyLen)}
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
			return append(s.out, 0xFF, markerEOI), nil
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

// nextMarker finds the first marker at or after pos and returns the offset of
// the 0xFF that immediately precedes its code. Bytes before it that are not a
// marker, which standard decoders ignore, are skipped and not copied.
func nextMarker(data []byte, pos int) (int, byte, error) {
	for {
		next := bytes.IndexByte(data[pos:], 0xFF)
		if next < 0 {
			return 0, 0, ErrMalformed
		}
		pos += next
		for pos+1 < len(data) && data[pos+1] == 0xFF {
			pos++
		}
		if pos+1 >= len(data) {
			return 0, 0, ErrMalformed
		}
		if data[pos+1] != 0x00 {
			return pos, data[pos+1], nil
		}
		pos += 2
	}
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
		if o := orientation(payload); !s.sawExif && o != upright {
			s.out = append(append(append(s.out, orientationOnlyAPP1...), exifHeader...), orientationOnlyTIFF(o)...)
		}
		// Only the first Exif segment's orientation counts, as for a viewer.
		s.sawExif = true
	case marker == markerAPP0:
		s.out = append(s.out, jfifWithoutThumbnail(payload)...)
	case marker == markerAPP2 && bytes.HasPrefix(payload, iccHeader),
		marker == markerAPP14 && bytes.HasPrefix(payload, adobeHeader):
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
