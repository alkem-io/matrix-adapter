package imagemeta

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/image/webp"
)

var webpColour = color.NRGBA{R: 10, G: 120, B: 230, A: 255}

const (
	webpFlagICC   = 0x20
	webpFlagAlpha = 0x10
)

func webpWithMetadata(o uint16) []byte {
	return riff(
		riffChunk("VP8X", vp8x(webpFlagICC|webpFlagAlpha|webpFlagEXIF|webpFlagXMP, 3, 2)),
		riffChunk("ICCP", fakeProfile),
		riffChunk("VP8L", vp8lSolid(3, 2, webpColour)),
		riffChunk("EXIF", exifTIFF(o)),
		riffChunk("XMP ", []byte("<x:xmpmeta>SecretXMP</x:xmpmeta>")),
		riffChunk("SECR", []byte("SecretPhone")),
	)
}

func requireDecodesToSolid(t *testing.T, data []byte) {
	t.Helper()
	img, err := webp.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 3, 2), img.Bounds())
	for y := range 2 {
		for x := range 3 {
			require.Equal(t, webpColour, color.NRGBAModel.Convert(img.At(x, y)))
		}
	}
}

func TestStripWebP_RemovesExifAndXMPAndFixesHeader(t *testing.T) {
	original := append(webpWithMetadata(1), "SecretTrailer"...)
	requireDecodesToSolid(t, original)

	out, err := Strip(FormatWebP, original)
	require.NoError(t, err)

	requireNoSecrets(t, out)
	chunks := riffChunks(t, out)
	require.Len(t, chunks, 3)
	require.Equal(t, []string{"VP8X", "ICCP", "VP8L"}, []string{chunks[0].kind, chunks[1].kind, chunks[2].kind})
	require.Equal(t, byte(webpFlagICC|webpFlagAlpha), chunks[0].payload[0], "EXIF and XMP flags cleared")
	require.Equal(t, fakeProfile, chunks[1].payload, "colour profile kept")
	requireDecodesToSolid(t, out)
}

func TestStripWebP_KeepsOnlyTheOrientationOfAnOrientedImage(t *testing.T) {
	out, err := Strip(FormatWebP, webpWithMetadata(6))
	require.NoError(t, err)

	requireNoSecrets(t, out)
	chunks := riffChunks(t, out)
	require.Len(t, chunks, 4)
	require.Equal(t, "EXIF", chunks[3].kind)
	require.Equal(t, orientationOnlyTIFF(6), chunks[3].payload)
	require.Equal(t, byte(webpFlagICC|webpFlagAlpha|webpFlagEXIF), chunks[0].payload[0])
	requireDecodesToSolid(t, out)
}

func TestStripWebP_RefusesMalformedContainers(t *testing.T) {
	image := riffChunk("VP8L", vp8lSolid(1, 1, webpColour))
	truncated := webpWithMetadata(1)
	for name, data := range map[string][]byte{
		"header only":            []byte("RIFF\x04\x00\x00\x00WEBP"),
		"RIFF size past the end": truncated[:len(truncated)-4],
		"chunk past RIFF end":    riff([]byte("VP8L\xFF\x00\x00\x00")),
		"no image chunk":         riff(riffChunk("EXIF", exifTIFF(1))),
		"short VP8X":             riff(riffChunk("VP8X", []byte{0, 0, 0}), image),
		"second VP8X":            riff(riffChunk("VP8X", vp8x(0, 1, 1)), riffChunk("VP8X", vp8x(0, 1, 1)), image),
	} {
		_, err := Strip(FormatWebP, data)
		require.ErrorIs(t, err, ErrMalformed, name)
	}
}
