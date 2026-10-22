package imagemeta

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/require"
)

// Fixtures are built in code so every privacy-relevant field is known and
// traceable: each one carries a "Secret…" marker the tests look for.

var (
	red   = color.RGBA{R: 255, A: 255}
	green = color.RGBA{G: 255, A: 255}
	blue  = color.RGBA{B: 255, A: 255}
	white = color.RGBA{R: 255, G: 255, B: 255, A: 255}

	secrets = [][]byte{
		[]byte("SecretPhone"), []byte("SecretXMP"), []byte("SecretIPTC"),
		[]byte("SecretComment"), []byte("SecretTrailer"), []byte("SecretText"),
		[]byte("SecretThumb"), gpsLatitude,
	}
	// gpsLatitude is 52°22'12.34" as three TIFF rationals.
	gpsLatitude  = []byte{52, 0, 0, 0, 1, 0, 0, 0, 22, 0, 0, 0, 1, 0, 0, 0, 0xD2, 0x04, 0, 0, 100, 0, 0, 0}
	fakeProfile  = []byte("fake-colour-profile-bytes")
	xmpNamespace = []byte("http://ns.adobe.com/xap/1.0/\x00")
)

// quadrants is a 32×16 image whose quarters are red, green / blue, white
// (top-left, top-right / bottom-left, bottom-right).
func quadrants() *image.RGBA {
	const w, h = 32, 16
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := red
			switch {
			case x >= w/2 && y < h/2:
				c = green
			case x < w/2 && y >= h/2:
				c = blue
			case x >= w/2 && y >= h/2:
				c = white
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// exifTIFF is a little-endian TIFF structure with a device make, an
// orientation and a GPS IFD holding a latitude.
func exifTIFF(o uint16) []byte {
	le := binary.LittleEndian
	entry := func(b []byte, tag, kind uint16, count, value uint32) []byte {
		b = le.AppendUint16(b, tag)
		b = le.AppendUint16(b, kind)
		b = le.AppendUint32(b, count)
		return le.AppendUint32(b, value)
	}
	b := []byte("II\x2A\x00\x08\x00\x00\x00")
	b = le.AppendUint16(b, 3)
	b = entry(b, 0x010F, 2, 12, 50) // Make, stored at 50
	b = entry(b, 0x0112, 3, 1, uint32(o))
	b = entry(b, 0x8825, 4, 1, 62) // GPS IFD at 62
	b = le.AppendUint32(b, 0)
	b = append(b, "SecretPhone\x00"...)
	b = le.AppendUint16(b, 2)
	b = entry(b, 0x0001, 2, 2, uint32('N'))
	b = entry(b, 0x0002, 5, 3, 92) // GPSLatitude at 92
	b = le.AppendUint32(b, 0)
	return append(b, gpsLatitude...)
}

// fixtureLen is the length of a fixture as stored in a container header.
// Fixtures are at most a few kilobytes.
func fixtureLen(b []byte) uint32 {
	return uint32(len(b)) //nolint:gosec // fixtures are at most a few kilobytes
}

func jpegSegment(marker byte, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	segment := binary.BigEndian.AppendUint32([]byte{0xFF, marker}, fixtureLen(body)+2)
	return append(append(segment[:2], segment[4:]...), body...) // keep the low 16 bits
}

func iccSegment() []byte {
	return jpegSegment(markerAPP2, iccHeader, []byte{1, 1}, fakeProfile)
}

// jpegWithMetadata encodes img and inserts every kind of metadata segment
// after SOI, plus a trailer after EOI.
func jpegWithMetadata(t *testing.T, img image.Image, o uint16) []byte {
	t.Helper()
	var encoded bytes.Buffer
	require.NoError(t, jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 100}))
	plain := encoded.Bytes()
	jfif := append(append([]byte{}, jfifHeader...), 1, 2, 1, 0, 72, 0, 72, 1, 1)
	segments := [][]byte{
		jpegSegment(markerAPP0, jfif, []byte("Sec")), // 1×1 thumbnail
		jpegSegment(markerAPP1, exifHeader, exifTIFF(o)),
		jpegSegment(markerAPP1, xmpNamespace, []byte("<x:xmpmeta>SecretXMP</x:xmpmeta>")),
		iccSegment(),
		jpegSegment(0xED, []byte("Photoshop 3.0\x008BIM SecretIPTC")),
		jpegSegment(markerAPP0, []byte("JFXX\x00\x10SecretThumb")),
		jpegSegment(markerCOM, []byte("SecretComment")),
	}
	out := append([]byte{}, plain[:2]...)
	for _, s := range segments {
		out = append(out, s...)
	}
	out = append(out, plain[2:]...)
	return append(out, "SecretTrailer"...)
}

// jpegMarkers lists the markers before the first scan.
func jpegMarkers(t *testing.T, data []byte) []byte {
	t.Helper()
	var markers []byte
	for pos := 2; ; {
		require.Equal(t, byte(0xFF), data[pos])
		marker := data[pos+1]
		markers = append(markers, marker)
		if marker == markerSOS {
			return markers
		}
		pos += 2 + int(binary.BigEndian.Uint16(data[pos+2:]))
	}
}

func pngChunkForTest(kind string, payload []byte) []byte {
	b := binary.BigEndian.AppendUint32(nil, fixtureLen(payload))
	b = append(b, kind...)
	b = append(b, payload...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(b[4:]))
}

// pngChunks lists chunk types in order.
func pngChunks(t *testing.T, data []byte) []string {
	t.Helper()
	require.True(t, bytes.HasPrefix(data, pngSignature))
	var kinds []string
	for pos := len(pngSignature); pos < len(data); {
		n := int(binary.BigEndian.Uint32(data[pos:]))
		kinds = append(kinds, string(data[pos+4:pos+8]))
		pos += 12 + n
	}
	return kinds
}

// bitWriter packs bits least-significant first, as VP8L requires.
type bitWriter struct {
	out   []byte
	nbits uint
}

func (w *bitWriter) write(value uint32, n uint) {
	for i := range n {
		if w.nbits%8 == 0 {
			w.out = append(w.out, 0)
		}
		w.out[len(w.out)-1] |= byte((value>>i)&1) << (w.nbits % 8)
		w.nbits++
	}
}

// vp8lSolid is a lossless VP8L bitstream of a w×h image in one colour: every
// prefix code is a one-symbol simple code, so each pixel costs zero bits.
func vp8lSolid(w, h uint32, c color.NRGBA) []byte {
	bw := &bitWriter{}
	bw.write(0x2F, 8)
	bw.write(w-1, 14)
	bw.write(h-1, 14)
	bw.write(1, 1) // alpha is used
	bw.write(0, 3) // version
	bw.write(0, 1) // no transform
	bw.write(0, 1) // no colour cache
	bw.write(0, 1) // no meta prefix codes
	for _, symbol := range []uint8{c.G, c.R, c.B, c.A, 0} {
		bw.write(1, 1) // simple code
		bw.write(0, 1) // one symbol
		bw.write(1, 1) // eight-bit symbol
		bw.write(uint32(symbol), 8)
	}
	return bw.out
}

func riffChunk(kind string, payload []byte) []byte {
	b := binary.LittleEndian.AppendUint32([]byte(kind), fixtureLen(payload))
	b = append(b, payload...)
	if len(payload)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

func riff(chunks ...[]byte) []byte {
	body := append([]byte("WEBP"), bytes.Join(chunks, nil)...)
	return append(binary.LittleEndian.AppendUint32([]byte("RIFF"), fixtureLen(body)), body...)
}

// vp8x is an extended-format header for a w×h canvas (24-bit sizes).
func vp8x(flags byte, w, h uint32) []byte {
	b := binary.LittleEndian.AppendUint32([]byte{flags, 0, 0, 0}, w-1)[:7]
	return binary.LittleEndian.AppendUint32(b, h-1)[:10]
}

type riffEntry struct {
	kind    string
	payload []byte
}

func riffChunks(t *testing.T, data []byte) []riffEntry {
	t.Helper()
	require.Equal(t, "RIFF", string(data[:4]))
	require.Equal(t, "WEBP", string(data[8:12]))
	require.Equal(t, len(data)-8, int(binary.LittleEndian.Uint32(data[4:])), "RIFF size must cover exactly the file")
	var entries []riffEntry
	for pos := 12; pos < len(data); {
		n := int(binary.LittleEndian.Uint32(data[pos+4:]))
		entries = append(entries, riffEntry{string(data[pos : pos+4]), data[pos+8 : pos+8+n]})
		pos += 8 + n + n%2
	}
	return entries
}

func requireNoSecrets(t *testing.T, data []byte) {
	t.Helper()
	for _, secret := range secrets {
		require.False(t, bytes.Contains(data, secret), "output still contains %q", secret)
	}
}
