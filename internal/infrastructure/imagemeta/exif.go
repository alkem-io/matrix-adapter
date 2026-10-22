package imagemeta

import (
	"bytes"
	"encoding/binary"
)

const (
	tagOrientation = 0x0112
	tiffTypeShort  = 3
	upright        = byte(1)
)

// exifHeader prefixes the TIFF structure inside a JPEG APP1 segment, and
// sometimes (non-standardly) inside a WebP EXIF chunk.
var exifHeader = []byte("Exif\x00\x00")

// orientation returns the Orientation tag of a TIFF structure's first IFD, or
// upright when it is absent or unreadable. An unreadable structure is not an
// error: it is discarded anyway, and the image itself is still well formed.
func orientation(tiff []byte) byte {
	tiff = bytes.TrimPrefix(tiff, exifHeader)
	if len(tiff) < 8 {
		return upright
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return upright
	}
	if order.Uint16(tiff[2:]) != 42 {
		return upright
	}
	ifd := uint64(order.Uint32(tiff[4:]))
	if ifd+2 > uint64(len(tiff)) {
		return upright
	}
	entries := uint64(order.Uint16(tiff[ifd:]))
	for i := range entries {
		entry := ifd + 2 + i*12
		if entry+12 > uint64(len(tiff)) {
			return upright
		}
		if order.Uint16(tiff[entry:]) != tagOrientation {
			continue
		}
		if order.Uint16(tiff[entry+2:]) != tiffTypeShort || order.Uint32(tiff[entry+4:]) != 1 {
			return upright
		}
		if value := order.Uint16(tiff[entry+8:]); value >= 1 && value <= 8 {
			return byte(value)
		}
		return upright
	}
	return upright
}

// orientationOnlyTIFF is a big-endian TIFF structure whose single IFD holds
// nothing but the Orientation tag. It lets a lossless rewrite keep how an image
// displays without keeping any other Exif field.
func orientationOnlyTIFF(o byte) []byte {
	return []byte{
		'M', 'M', 0x00, 0x2A, // byte order, TIFF magic
		0x00, 0x00, 0x00, 0x08, // offset of IFD0
		0x00, 0x01, // one entry
		0x01, 0x12, 0x00, tiffTypeShort, 0x00, 0x00, 0x00, 0x01, 0x00, o, 0x00, 0x00, // Orientation = o
		0x00, 0x00, 0x00, 0x00, // no next IFD
	}
}
