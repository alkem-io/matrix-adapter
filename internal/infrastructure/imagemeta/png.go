package imagemeta

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
)

// pngKept lists the chunks needed to render a PNG faithfully: the critical
// chunks, transparency, colour space and profile, physical pixel size, and
// the animated-PNG chunks. Every other chunk (eXIf, tEXt, iTXt, zTXt, tIME and
// any private or unknown chunk) is dropped.
var pngKept = map[string]bool{
	"IHDR": true, "PLTE": true, "IDAT": true, "IEND": true,
	"tRNS": true, "bKGD": true, "sBIT": true, "pHYs": true,
	"gAMA": true, "cHRM": true, "sRGB": true, "iCCP": true, "cICP": true,
	"mDCV": true, "mDCv": true, "cLLI": true, "cLLi": true,
	"acTL": true, "fcTL": true, "fdAT": true,
}

// stripPNG rewrites a PNG losslessly, keeping only the chunks in pngKept.
// Pixel data is copied untouched. An Exif orientation other than upright is
// kept as an orientation-only eXIf chunk at the original position, so the
// image displays exactly as before wherever eXIf orientation was honoured.
func stripPNG(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, pngSignature) {
		return nil, ErrMalformed
	}
	out := append(make([]byte, 0, len(data)), pngSignature...)
	exifAt, o, sawData := -1, upright, false
	for pos := uint64(len(pngSignature)); ; {
		if pos+12 > uint64(len(data)) {
			return nil, ErrMalformed
		}
		length := uint64(binary.BigEndian.Uint32(data[pos:]))
		end := pos + 12 + length
		if length > 1<<31-1 || end > uint64(len(data)) {
			return nil, ErrMalformed
		}
		kind := string(data[pos+4 : pos+8])
		if (pos == uint64(len(pngSignature))) != (kind == "IHDR") {
			return nil, ErrMalformed
		}
		switch {
		case kind == "eXIf" && exifAt < 0:
			exifAt, o = len(out), orientation(data[pos+8:end-4])
		case pngKept[kind]:
			out = append(out, data[pos:end]...)
		}
		sawData = sawData || kind == "IDAT"
		pos = end
		if kind != "IEND" {
			continue
		}
		if !sawData {
			return nil, ErrMalformed
		}
		if o != upright {
			out = splice(out, exifAt, pngChunk("eXIf", orientationOnlyTIFF(o)))
		}
		return out, nil
	}
}

func pngChunk(kind string, payload []byte) []byte {
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(payload))) //nolint:gosec // payload is a fixed 26-byte structure
	chunk = append(chunk, kind...)
	chunk = append(chunk, payload...)
	return binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
}
