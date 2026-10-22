package imagemeta

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
)

const (
	markerTEM   = 0x01
	markerSOF0  = 0xC0
	markerSOF1  = 0xC1
	markerSOF2  = 0xC2
	markerSOF15 = 0xCF
	markerDHT   = 0xC4
	markerJPG   = 0xC8
	markerDAC   = 0xCC
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
	// maxUprightPixels bounds the work physical rotation does. Above it the
	// image keeps an orientation-only Exif segment instead.
	maxUprightPixels = 24_000_000
)

// MaxRotationBytes is the most memory one JPEG rotation may be charged. The
// charge is estimated from the frame header before anything is decoded, and
// an image whose estimate is higher keeps an orientation-only Exif segment
// instead, so a small file that decodes to far more than its pixel count
// suggests (progressive coding, four components, full-resolution chroma)
// cannot exhaust the service's memory. A 24 MP baseline photo with the usual
// 4:2:0 chroma fits.
const MaxRotationBytes int64 = 160 << 20

// Budget is the memory a JPEG rotation is charged against while it decodes,
// rotates and re-encodes; *semaphore.Weighted satisfies it. Acquire is never
// asked for more than MaxRotationBytes, and its error is returned by Strip.
type Budget interface {
	// Acquire charges n bytes, waiting while ctx allows; an error means the
	// rotation must not run.
	Acquire(ctx context.Context, n int64) error
	// Release returns n bytes charged by a successful Acquire.
	Release(n int64)
}

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
	// frameMarker and frame are the first frame header's marker and payload.
	frameMarker byte
	frame       []byte
	// adobeRGB is set when an Adobe segment says the samples are not YCbCr.
	adobeRGB bool
}

// stripJPEG keeps the segments needed to render (tables, frame, scans, JFIF
// header without its thumbnail, ICC profile, Adobe colour transform) and drops
// every other application segment and comment: Exif (location, device, time,
// thumbnail), XMP, IPTC/Photoshop, multi-picture data, and anything appended
// after the end-of-image marker.
func stripJPEG(ctx context.Context, data []byte, budget Budget) ([]byte, error) {
	s, err := scanJPEG(data)
	if err != nil {
		return nil, err
	}
	if s.orientation == upright {
		return s.out, nil
	}
	rotated, ok, err := s.upright(ctx, budget)
	if err != nil {
		return nil, err
	}
	if ok {
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
		// The colour transform byte, as the standard decoder reads it.
		s.adobeRGB = len(payload) >= 12 && payload[11] == 0
	case marker >= markerAPP0 && marker <= markerAPP15, marker == markerCOM:
		// Every other application segment and every comment is metadata.
	case isFrameHeader(marker):
		if s.frameMarker == 0 {
			s.frameMarker, s.frame = marker, payload
		}
		s.out = append(s.out, segment...)
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

func isFrameHeader(marker byte) bool {
	return marker >= markerSOF0 && marker <= markerSOF15 &&
		marker != markerDHT && marker != markerJPG && marker != markerDAC
}

// rotationCost estimates, from the frame header, the memory the standard
// decoder, orient and the re-encode hold at once for this image. ok is false
// when the image is not to be rotated: a coding the decoder does not support,
// more than maxUprightPixels, or an estimate above MaxRotationBytes.
func (s *jpegScan) rotationCost() (cost int64, ok bool) {
	pixels, samples, components, ok := s.frameSize()
	if !ok {
		return 0, false
	}
	cost = samples
	if s.frameMarker == markerSOF2 {
		// A progressive decode also holds a 64×int32 coefficient block for
		// every 8×8 block of samples.
		cost += 4 * samples
	}
	switch {
	case components == 1:
		cost += pixels // grey destination
	case components == 3 && !s.rgb():
		cost += 3 * pixels // full-resolution YCbCr destination
	default:
		cost += 8 * pixels // converted to RGBA or CMYK, then an RGBA destination
	}
	// The re-encode buffer, pre-sized to a byte per pixel, and the copy that
	// carries the colour profile.
	cost += 2 * pixels
	return cost, cost <= MaxRotationBytes
}

// frameSize reads the frame header: the pixel and component counts, and the
// samples the decoder holds, which is every component at its own sampling in
// whole minimum coded units (a single component is never subsampled). ok is
// false for a frame the decoder does not support or one over maxUprightPixels.
func (s *jpegScan) frameSize() (pixels, samples int64, components int, ok bool) {
	f := s.frame
	switch {
	case s.frameMarker != markerSOF0 && s.frameMarker != markerSOF1 && s.frameMarker != markerSOF2,
		len(f) < 6, f[0] != 8: // the decoder reads 8-bit samples only
		return 0, 0, 0, false
	}
	height, width, components := int64(f[1])<<8|int64(f[2]), int64(f[3])<<8|int64(f[4]), int(f[5])
	pixels = width * height
	if (components != 1 && components != 3 && components != 4) || len(f) < 6+3*components ||
		pixels == 0 || pixels > maxUprightPixels {
		return 0, 0, 0, false
	}
	hMax, vMax, units, ok := sampling(f[6:], components)
	if !ok {
		return 0, 0, 0, false
	}
	mcus := (width + 8*hMax - 1) / (8 * hMax) * ((height + 8*vMax - 1) / (8 * vMax))
	return pixels, 64 * mcus * units, components, true
}

// sampling reads the components' sampling factors: the largest horizontal
// and vertical factors, which size a minimum coded unit, and the 8×8 blocks
// each unit holds across all components.
func sampling(specs []byte, components int) (hMax, vMax, units int64, ok bool) {
	hMax, vMax = 1, 1
	for i := range components {
		h, v := int64(specs[1+3*i]>>4), int64(specs[1+3*i]&0x0F)
		if components == 1 {
			h, v = 1, 1
		}
		if h < 1 || h > 4 || v < 1 || v > 4 {
			return 0, 0, 0, false
		}
		hMax, vMax, units = max(hMax, h), max(vMax, v), units+h*v
	}
	return hMax, vMax, units, true
}

// rgb reports whether three components may hold RGB rather than YCbCr, which
// the decoder converts to a 4-byte-per-pixel image. It errs towards RGB.
func (s *jpegScan) rgb() bool {
	f := s.frame
	return s.adobeRGB || (f[6] == 'R' && f[9] == 'G' && f[12] == 'B')
}

// upright decodes, rotates and re-encodes the stripped stream so the pixels
// themselves face the right way. The estimated memory is charged to budget
// for the whole rotation; a refused charge or a done ctx is returned as the
// error. ok is false, with no error, when the image is not to be rotated or
// uses a coding the standard decoder does not support.
func (s *jpegScan) upright(ctx context.Context, budget Budget) (out []byte, ok bool, err error) {
	cost, ok := s.rotationCost()
	if !ok {
		return nil, false, nil
	}
	if err := budget.Acquire(ctx, cost); err != nil {
		return nil, false, err
	}
	defer budget.Release(cost)
	// Acquire can succeed on a context that is already done.
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	img, err := jpeg.Decode(untilDone(ctx, bytes.NewReader(s.out)))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, false, ctxErr
		}
		return nil, false, nil
	}
	// A profile describes YCbCr or grey samples; a CMYK source is converted
	// to RGB on the way through, so its CMYK profile would no longer apply.
	var profile [][]byte
	switch img.(type) {
	case *image.YCbCr, *image.Gray:
		profile = s.profile
	}
	rotated := orient(img, s.orientation)
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	var encoded bytes.Buffer
	encoded.Grow(rotated.Bounds().Dx() * rotated.Bounds().Dy())
	if err := jpeg.Encode(&encoded, rotated, &jpeg.Options{Quality: uprightQuality}); err != nil {
		return nil, false, nil
	}
	body := encoded.Bytes()
	out = make([]byte, 0, len(body)+profileSize(profile))
	out = append(out, body[:2]...)
	for _, segment := range profile {
		out = append(out, segment...)
	}
	return append(out, body[2:]...), true, nil
}

// untilDone reads r until ctx is done, which stops the decode reading it.
func untilDone(ctx context.Context, r io.Reader) io.Reader {
	return readFunc(func(p []byte) (int, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		return r.Read(p)
	})
}

type readFunc func([]byte) (int, error)

func (f readFunc) Read(p []byte) (int, error) { return f(p) }

func profileSize(segments [][]byte) int {
	size := 0
	for _, segment := range segments {
		size += len(segment)
	}
	return size
}
