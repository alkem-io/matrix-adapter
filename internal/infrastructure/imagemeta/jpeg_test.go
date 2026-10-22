package imagemeta

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
)

func TestStripJPEG_RemovesMetadataLosslesslyWhenUpright(t *testing.T) {
	for _, o := range []uint16{0, 1} { // absent-equivalent (invalid) and upright
		original := jpegWithMetadata(t, quadrants(32, 16), o)

		out, err := Strip(t.Context(), FormatJPEG, original, ample())
		require.NoError(t, err)

		requireNoSecrets(t, out)
		require.Equal(t, []byte{markerAPP0, markerAPP2, 0xDB, 0xC0, 0xC4, markerSOS}, jpegMarkers(t, out),
			"only the thumbnail-free JFIF header, the ICC profile and the image structure remain")
		require.True(t, bytes.Contains(out, iccSegment()), "colour profile kept verbatim")
		jfif := append(append([]byte{0xFF, markerAPP0, 0x00, 0x10}, jfifHeader...), 1, 2, 1, 0, 72, 0, 72, 0, 0)
		require.True(t, bytes.HasPrefix(out[2:], jfif), "JFIF header kept with its thumbnail removed")
		require.True(t, bytes.HasSuffix(out, []byte{0xFF, markerEOI}), "nothing after end of image")

		want, err := jpeg.Decode(bytes.NewReader(original))
		require.NoError(t, err)
		got, err := jpeg.Decode(bytes.NewReader(out))
		require.NoError(t, err)
		require.Equal(t, want, got, "segment removal must not change a single sample")
	}
}

func TestStripJPEG_RotatesOrientedImageUpright(t *testing.T) {
	// Expected upright quadrants (top-left, top-right, bottom-left,
	// bottom-right) for a stored image whose quadrants are red, green /
	// blue, white, by the meaning of each Exif Orientation value.
	for o, want := range map[uint16][4]color.RGBA{
		2: {green, red, white, blue},
		3: {white, blue, green, red},
		4: {blue, white, red, green},
		5: {red, blue, green, white},
		6: {blue, red, white, green},
		7: {white, green, blue, red},
		8: {green, white, red, blue},
	} {
		original := jpegWithMetadata(t, quadrants(64, 32), o)

		out, err := Strip(t.Context(), FormatJPEG, original, ample())
		require.NoError(t, err, "orientation %d", o)

		requireNoSecrets(t, out)
		require.NotContains(t, jpegMarkers(t, out), byte(markerAPP1), "no Exif left to re-orient the image")
		require.True(t, bytes.HasPrefix(out[2:], iccSegment()), "colour profile carried into the re-encode")
		img, err := jpeg.Decode(bytes.NewReader(out))
		require.NoError(t, err)
		w, h := 64, 32
		if o >= 5 {
			w, h = 32, 64
		}
		require.Equal(t, image.Rect(0, 0, w, h), img.Bounds(), "orientation %d", o)
		corners := [4]image.Point{{w / 4, h / 4}, {3 * w / 4, h / 4}, {w / 4, 3 * h / 4}, {3 * w / 4, 3 * h / 4}}
		for i, p := range corners {
			requireNear(t, want[i], img.At(p.X, p.Y), "orientation %d quadrant %d", o, i)
		}
	}
}

func TestStripJPEG_KeepsOnlyOrientationWhenTooLargeToRotate(t *testing.T) {
	original := jpegWithMetadata(t, quadrants(16, 16), 6)
	// Declare a 6000×6000 frame: beyond the decode bound, so the pixels are
	// never decoded and the orientation survives as the only Exif field.
	sof := bytes.Index(original, []byte{0xFF, 0xC0})
	require.Positive(t, sof)
	binary.BigEndian.PutUint16(original[sof+5:], 6000)
	binary.BigEndian.PutUint16(original[sof+7:], 6000)

	out, err := Strip(t.Context(), FormatJPEG, original, refusing{t})
	require.NoError(t, err)

	requireNoSecrets(t, out)
	require.True(t, bytes.Contains(out, orientationOnlyExif(6)))
	require.Equal(t, byte(6), orientation(orientationOnlyExif(6)[4:]))
}

func orientationOnlyExif(o byte) []byte {
	return append(append([]byte{0xFF, markerAPP1, 0x00, 0x22}, exifHeader...), orientationOnlyTIFF(o)...)
}

var (
	yCbCr420 = []component{{1, 2, 2}, {2, 1, 1}, {3, 1, 1}}
	yCbCr444 = []component{{1, 1, 1}, {2, 1, 1}, {3, 1, 1}}
	rgb420   = []component{{'R', 2, 2}, {'G', 1, 1}, {'B', 1, 1}}
	cmyk     = []component{{'C', 1, 1}, {'M', 1, 1}, {'Y', 1, 1}, {'K', 1, 1}}
	grey     = []component{{1, 2, 2}}
)

func TestRotationCost_EstimatesTheDecodeNotThePixelCount(t *testing.T) {
	fixture := jpegWithMetadata(t, quadrants(16, 16), 6)
	for name, tc := range map[string]struct {
		marker     byte
		w, h       uint16
		components []component
		cost       int64
		ok         bool
	}{
		"24 MP baseline 4:2:0 photo": {markerSOF0, 6000, 4000, yCbCr420, 156_000_000, true},
		"12 MP progressive 4:2:0":    {markerSOF2, 4032, 3024, yCbCr420, 152_409_600, true},
		"24 MP extended grey":        {markerSOF1, 6000, 4000, grey, 96_000_000, true},
		"24 MP progressive 4:2:0":    {markerSOF2, 6000, 4000, yCbCr420, 300_000_000, false},
		"24 MP baseline 4:4:4":       {markerSOF0, 6000, 4000, yCbCr444, 192_000_000, false},
		"24 MP baseline RGB":         {markerSOF0, 6000, 4000, rgb420, 276_000_000, false},
		"24 MP progressive CMYK":     {markerSOF2, 6000, 4000, cmyk, 720_000_000, false},
		"over 24 MP":                 {markerSOF0, 6000, 4001, grey, 0, false},
		"lossless coding":            {0xC3, 64, 64, yCbCr420, 0, false},
		"two components":             {markerSOF0, 64, 64, yCbCr420[:2], 0, false},
	} {
		s, err := scanJPEG(withFrame(t, fixture, tc.marker, tc.w, tc.h, tc.components...))
		require.NoError(t, err, name)
		cost, ok := s.rotationCost()
		require.Equal(t, tc.ok, ok, name)
		if tc.cost > 0 {
			require.Equal(t, tc.cost, cost, name)
		}
	}
	twelveBit := withFrame(t, fixture, markerSOF1, 64, 64, grey...)
	twelveBit[bytes.Index(twelveBit, []byte{0xFF, markerSOF1})+4] = 12
	s, err := scanJPEG(twelveBit)
	require.NoError(t, err)
	_, ok := s.rotationCost()
	require.False(t, ok, "12-bit samples")
}

func TestStripJPEG_KeepsOnlyOrientationWhenTheDecodeWouldCostTooMuch(t *testing.T) {
	fixture := jpegWithMetadata(t, quadrants(16, 16), 6)
	// Each is at most 24 MP but decodes to several times its pixel count; the
	// same frame as YCbCr 4:2:0 baseline would be rotated.
	for name, data := range map[string][]byte{
		"progressive 4:4:4": withFrame(t, fixture, markerSOF2, 4896, 4896, yCbCr444...),
		"progressive CMYK":  withFrame(t, fixture, markerSOF2, 4896, 4896, cmyk...),
		"baseline CMYK":     withFrame(t, fixture, markerSOF0, 4896, 4896, cmyk...),
		"baseline RGB":      withFrame(t, fixture, markerSOF0, 6000, 4000, rgb420...),
		"Adobe RGB": withFrame(t, append(append([]byte{0xFF, markerSOI},
			jpegSegment(markerAPP14, adobeHeader, []byte{0, 100, 0, 0, 0, 0, 0})...), fixture[2:]...),
			markerSOF0, 6000, 4000, yCbCr420...),
	} {
		out, err := Strip(t.Context(), FormatJPEG, data, refusing{t})
		require.NoError(t, err, name)
		requireNoSecrets(t, out)
		require.True(t, bytes.Contains(out, orientationOnlyExif(6)), name)
	}
}

func TestStripJPEG_ChargesTheRotationToTheBudget(t *testing.T) {
	oriented := jpegWithMetadata(t, quadrants(64, 32), 6)
	s, err := scanJPEG(oriented)
	require.NoError(t, err)
	cost, ok := s.rotationCost()
	require.True(t, ok)
	budget := &recording{}

	_, err = Strip(t.Context(), FormatJPEG, oriented, budget)
	require.NoError(t, err)

	require.Equal(t, cost, budget.acquired)
	require.Equal(t, cost, budget.released, "the charge is returned once the rotation is done")

	upright := &recording{}
	_, err = Strip(t.Context(), FormatJPEG, jpegWithMetadata(t, quadrants(64, 32), 1), upright)
	require.NoError(t, err)
	require.Zero(t, upright.acquired, "an upright image is not charged")
}

func TestStripJPEG_ReturnsARefusedRotationAsAnError(t *testing.T) {
	full := semaphore.NewWeighted(MaxRotationBytes)
	require.True(t, full.TryAcquire(MaxRotationBytes))
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	out, err := Strip(ctx, FormatJPEG, jpegWithMetadata(t, quadrants(64, 32), 6), full)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, out, "a refused rotation never falls back to the unrotated image")

	_, err = Strip(t.Context(), FormatJPEG, jpegWithMetadata(t, quadrants(64, 32), 1), full)
	require.NoError(t, err, "an image that needs no rotation does not wait for the budget")
}

func TestStripJPEG_DoesNotDecodeOnceTheContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	// The charge succeeds, but the request is gone by the time it does.
	budget := &recording{onAcquire: cancel}

	out, err := Strip(ctx, FormatJPEG, jpegWithMetadata(t, quadrants(64, 32), 6), budget)

	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, out)
	require.Equal(t, budget.acquired, budget.released)
}

func TestUntilDone_StopsReadingOnceTheContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	r := untilDone(ctx, strings.NewReader("abcd"))
	buf := make([]byte, 2)
	n, err := r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	cancel()
	_, err = r.Read(buf)
	require.ErrorIs(t, err, context.Canceled)
}

// refusing is a rotation budget that fails the test if it is ever charged.
type refusing struct{ t *testing.T }

func (r refusing) Acquire(context.Context, int64) error {
	r.t.Error("a rotation was charged")
	return errors.New("refused")
}

func (refusing) Release(int64) {}

// recording is a rotation budget that admits every charge and records it.
type recording struct {
	acquired, released int64
	onAcquire          func()
}

func (r *recording) Acquire(_ context.Context, n int64) error {
	r.acquired += n
	if r.onAcquire != nil {
		r.onAcquire()
	}
	return nil
}

func (r *recording) Release(n int64) { r.released += n }

func TestStripJPEG_RefusesMalformedContainers(t *testing.T) {
	valid := jpegWithMetadata(t, quadrants(16, 16), 1)
	sos := bytes.Index(valid, []byte{0xFF, markerSOS})
	for name, data := range map[string][]byte{
		"only SOI":                 {0xFF, markerSOI, 0xFF},
		"segment past the end":     {0xFF, markerSOI, 0xFF, markerAPP1, 0x40, 0x00, 'E', 'x', 'i', 'f'},
		"segment length too small": {0xFF, markerSOI, 0xFF, markerCOM, 0x00, 0x01, 0xFF, markerEOI},
		"no scan before EOI":       {0xFF, markerSOI, 0xFF, markerCOM, 0x00, 0x03, 'x', 0xFF, markerEOI},
		"truncated scan":           valid[:sos+40],
		"garbage between segments": append(append([]byte{}, valid[:20]...), 0x00, 0x01, 0x02),
		"second SOI":               {0xFF, markerSOI, 0xFF, markerSOI},
	} {
		_, err := Strip(t.Context(), FormatJPEG, data, ample())
		require.ErrorIs(t, err, ErrMalformed, name)
	}
}

func TestOrientation_ReadsOnlyAWellFormedTag(t *testing.T) {
	require.Equal(t, byte(6), orientation(exifTIFF(6)))
	require.Equal(t, byte(8), orientation(append(append([]byte{}, exifHeader...), orientationOnlyTIFF(8)...)))
	for name, tiff := range map[string][]byte{
		"empty":         nil,
		"bad order":     []byte("XX\x2A\x00\x08\x00\x00\x00"),
		"IFD past end":  []byte("II\x2A\x00\xFF\x00\x00\x00"),
		"out of range":  exifTIFF(9),
		"entry cut off": exifTIFF(6)[:20],
	} {
		require.Equal(t, upright, orientation(tiff), name)
	}
}

func requireNear(t *testing.T, want color.RGBA, got color.Color, msgAndArgs ...any) {
	t.Helper()
	r, g, b, _ := got.RGBA()
	for i, pair := range [][2]uint32{{uint32(want.R), r >> 8}, {uint32(want.G), g >> 8}, {uint32(want.B), b >> 8}} {
		diff := int(pair[0]) - int(pair[1])
		require.LessOrEqual(t, diff*diff, 24*24, append(msgAndArgs, "channel", i, "want", want, "got", got)...)
	}
}
