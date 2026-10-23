package imagemeta

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// vp8lPayload stands in for a lossless bitstream; the rewrite never reads it.
var vp8lPayload = []byte("\x2fplaceholder VP8L bitstream")

const (
	webpFlagICC   = 0x20
	webpFlagAlpha = 0x10
)

func webpWithMetadata(o uint16) []byte {
	return riff(
		riffChunk("VP8X", vp8x(webpFlagICC|webpFlagAlpha|webpFlagEXIF|webpFlagXMP, 3, 2)),
		riffChunk("ICCP", fakeProfile),
		riffChunk("VP8L", vp8lPayload),
		riffChunk("EXIF", exifTIFF(o)),
		riffChunk("XMP ", []byte("<x:xmpmeta>SecretXMP</x:xmpmeta>")),
		riffChunk("SECR", []byte("SecretPhone")),
	)
}

func TestStripWebP_RemovesExifAndXMPAndFixesHeader(t *testing.T) {
	original := append(webpWithMetadata(1), "SecretTrailer"...)

	out, err := Strip(FormatWebP, original)
	require.NoError(t, err)

	requireNoSecrets(t, out)
	chunks := riffChunks(t, out)
	require.Len(t, chunks, 3)
	require.Equal(t, []string{"VP8X", "ICCP", "VP8L"}, []string{chunks[0].kind, chunks[1].kind, chunks[2].kind})
	require.Equal(t, byte(webpFlagICC|webpFlagAlpha), chunks[0].payload[0], "EXIF and XMP flags cleared")
	require.Equal(t, fakeProfile, chunks[1].payload, "colour profile kept")
	require.Equal(t, vp8lPayload, chunks[2].payload, "bitstream kept verbatim")
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
}

func TestStripWebP_RefusesMalformedContainers(t *testing.T) {
	image := riffChunk("VP8L", vp8lPayload)
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
