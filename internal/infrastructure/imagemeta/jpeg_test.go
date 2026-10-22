package imagemeta

import (
	"bytes"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripJPEG_RemovesMetadataLosslesslyWhenUpright(t *testing.T) {
	for _, o := range []uint16{0, 1} { // absent-equivalent (invalid) and upright
		original := jpegWithMetadata(t, quadrants(), o)

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

func TestStripJPEG_KeepsOnlyTheOrientationOfAnOrientedImage(t *testing.T) {
	for o := byte(2); o <= 8; o++ {
		original := jpegWithMetadata(t, quadrants(), uint16(o))

		out, err := Strip(FormatJPEG, original)
		require.NoError(t, err, "orientation %d", o)

		requireNoSecrets(t, out)
		require.Equal(t, []byte{markerAPP0, markerAPP1, markerAPP2, 0xDB, 0xC0, 0xC4, markerSOS}, jpegMarkers(t, out),
			"the orientation-only Exif segment stands where the Exif segment stood")
		require.True(t, bytes.Contains(out, orientationOnlyExif(o)), "orientation %d", o)
		require.Equal(t, len(original)+len(orientationOnlyExif(o)), cap(out), "the rewrite is never copied to grow it")

		want, err := jpeg.Decode(bytes.NewReader(original))
		require.NoError(t, err)
		got, err := jpeg.Decode(bytes.NewReader(out))
		require.NoError(t, err)
		require.Equal(t, want, got, "the pixels are not touched")
	}
}

func orientationOnlyExif(o byte) []byte {
	return append(append([]byte{0xFF, markerAPP1, 0x00, 0x22}, exifHeader...), orientationOnlyTIFF(o)...)
}

func TestStripJPEG_RefusesMalformedContainers(t *testing.T) {
	valid := jpegWithMetadata(t, quadrants(), 1)
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
