package imagemeta

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSniff_IdentifiesContainersFromBytesAlone(t *testing.T) {
	for name, tc := range map[string]struct {
		head   string
		format Format
	}{
		"empty":         {"", FormatNone},
		"JPEG":          {"\xFF\xD8\xFF\xE0", FormatJPEG},
		"PNG":           {"\x89PNG\r\n\x1a\n\x00\x00", FormatPNG},
		"WebP":          {"RIFF\x10\x20\x30\x40WEBP", FormatWebP},
		"RIFF but WAVE": {"RIFF\x10\x20\x30\x40WAVE", FormatNone},
		"GIF":           {"GIF89a\x01\x00", FormatNone},
		"HEIC":          {"\x00\x00\x00\x18ftypheic", FormatNone},
		"TIFF":          {"II*\x00\x08\x00", FormatNone},
		"PDF":           {"%PDF-1.7", FormatNone},
		"text":          {"half", FormatNone},
	} {
		require.Equal(t, tc.format, Sniff([]byte(tc.head)), name)
	}
}
