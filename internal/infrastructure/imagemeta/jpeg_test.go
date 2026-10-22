package imagemeta

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStripJPEG_RemovesMetadataLosslesslyWhenUpright(t *testing.T) {
	for _, o := range []uint16{0, 1} { // absent-equivalent (invalid) and upright
		original := jpegWithMetadata(t, quadrants(32, 16), o)

		out, err := Strip(FormatJPEG, original)
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

		out, err := Strip(FormatJPEG, original)
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

	out, err := Strip(FormatJPEG, original)
	require.NoError(t, err)

	requireNoSecrets(t, out)
	exif := append(append([]byte{0xFF, markerAPP1, 0x00, 0x22}, exifHeader...), orientationOnlyTIFF(6)...)
	require.True(t, bytes.Contains(out, exif))
	require.Equal(t, byte(6), orientation(exif[4:]))
}

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
		_, err := Strip(FormatJPEG, data)
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

func TestStripJPEG_RotatesOneImageAtATime(t *testing.T) {
	oriented := jpegWithMetadata(t, quadrants(64, 32), 6)
	upright := jpegWithMetadata(t, quadrants(64, 32), 1)
	uprightGate.Lock()
	rotated := make(chan error, 1)
	go func() {
		_, err := Strip(FormatJPEG, oriented)
		rotated <- err
	}()

	_, err := Strip(FormatJPEG, upright)
	require.NoError(t, err, "an image that needs no rotation does not wait")
	select {
	case <-rotated:
		uprightGate.Unlock()
		t.Fatal("a rotation ran while another held the decode")
	case <-time.After(100 * time.Millisecond):
	}

	uprightGate.Unlock()
	select {
	case err := <-rotated:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("rotation did not resume once the decode was free")
	}
}
