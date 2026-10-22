package imagemeta

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
)

// pngWithMetadata encodes a translucent image and inserts textual, time, Exif
// and private chunks after IHDR, plus a trailer after IEND.
func pngWithMetadata(t *testing.T, o uint16) ([]byte, image.Image) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	for y := range uint8(6) {
		for x := range uint8(8) {
			img.SetNRGBA(int(x), int(y), color.NRGBA{R: x * 30, G: y * 40, B: 200, A: 100 + x*10})
		}
	}
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, img))
	plain := encoded.Bytes()
	ihdrEnd := len(pngSignature) + 12 + 13
	out := append([]byte{}, plain[:ihdrEnd]...)
	for _, chunk := range [][]byte{
		pngChunkForTest("iCCP", append([]byte("profile\x00\x00"), fakeProfile...)),
		pngChunkForTest("eXIf", exifTIFF(o)),
		pngChunkForTest("tEXt", []byte("Comment\x00SecretText")),
		pngChunkForTest("zTXt", []byte("Author\x00\x00SecretText")),
		pngChunkForTest("iTXt", []byte("XML:com.adobe.xmp\x00\x00\x00\x00\x00SecretXMP")),
		pngChunkForTest("tIME", []byte{0x07, 0xEA, 10, 22, 12, 0, 0}),
		pngChunkForTest("prVt", []byte("SecretPhone")),
	} {
		out = append(out, chunk...)
	}
	out = append(out, plain[ihdrEnd:]...)
	return append(out, "SecretTrailer"...), img
}

func TestStripPNG_RemovesMetadataAndKeepsPixelsAndProfile(t *testing.T) {
	original, img := pngWithMetadata(t, 1)

	out, err := Strip(t.Context(), FormatPNG, original, ample())
	require.NoError(t, err)

	requireNoSecrets(t, out)
	require.Equal(t, []string{"IHDR", "iCCP", "IDAT", "IEND"}, pngChunks(t, out))
	require.True(t, bytes.Contains(out, fakeProfile), "colour profile kept")
	decoded, err := png.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	require.Equal(t, img, decoded, "pixels identical")
}

func TestStripPNG_KeepsOnlyTheOrientationOfAnOrientedImage(t *testing.T) {
	original, img := pngWithMetadata(t, 6)

	out, err := Strip(t.Context(), FormatPNG, original, ample())
	require.NoError(t, err)

	requireNoSecrets(t, out)
	require.Equal(t, []string{"IHDR", "iCCP", "eXIf", "IDAT", "IEND"}, pngChunks(t, out), "eXIf stays where it was")
	require.True(t, bytes.Contains(out, pngChunkForTest("eXIf", orientationOnlyTIFF(6))))
	decoded, err := png.Decode(bytes.NewReader(out)) // also verifies every chunk CRC
	require.NoError(t, err)
	require.Equal(t, img, decoded)
}

func TestStripPNG_RefusesMalformedContainers(t *testing.T) {
	valid, _ := pngWithMetadata(t, 1)
	ihdr := pngChunkForTest("IHDR", valid[len(pngSignature)+8:len(pngSignature)+8+13])
	iend := pngChunkForTest("IEND", nil)
	join := func(parts ...[]byte) []byte { return bytes.Join(append([][]byte{pngSignature}, parts...), nil) }
	for name, data := range map[string][]byte{
		"signature only":   pngSignature,
		"no IEND":          valid[:len(valid)-len("SecretTrailer")-12],
		"length past end":  join(ihdr, []byte{0x00, 0x10, 0x00, 0x00, 'I', 'D', 'A', 'T'}),
		"IHDR not first":   join(pngChunkForTest("tEXt", []byte("a\x00b")), ihdr, iend),
		"no image data":    join(ihdr, iend),
		"second IHDR":      join(ihdr, ihdr, iend),
		"oversized length": join(ihdr, []byte{0xFF, 0xFF, 0xFF, 0xFF, 'I', 'D', 'A', 'T', 0, 0, 0, 0}),
	} {
		_, err := Strip(t.Context(), FormatPNG, data, ample())
		require.ErrorIs(t, err, ErrMalformed, name)
	}
}
