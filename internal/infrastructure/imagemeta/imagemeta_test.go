package imagemeta

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSniff_IdentifiesContainersFromBytesAlone(t *testing.T) {
	for name, tc := range map[string]struct {
		head    string
		format  Format
		decided bool
	}{
		"empty":              {"", FormatNone, false},
		"JPEG":               {"\xFF\xD8\xFF\xE0", FormatJPEG, true},
		"JPEG prefix":        {"\xFF\xD8", FormatNone, false},
		"PNG":                {"\x89PNG\r\n\x1a\n\x00\x00", FormatPNG, true},
		"PNG prefix":         {"\x89PNG", FormatNone, false},
		"WebP":               {"RIFF\x10\x20\x30\x40WEBP", FormatWebP, true},
		"WebP prefix":        {"RIFF\xFF\xFF", FormatNone, false},
		"RIFF but WAVE":      {"RIFF\x10\x20\x30\x40WAVE", FormatNone, true},
		"GIF":                {"GIF89a\x01\x00", FormatNone, true},
		"HEIC":               {"\x00\x00\x00\x18ftypheic", FormatNone, true},
		"TIFF":               {"II*\x00\x08\x00", FormatNone, true},
		"PDF":                {"%PDF-1.7", FormatNone, true},
		"text":               {"half", FormatNone, true},
		"one byte non-image": {"h", FormatNone, true},
	} {
		format, decided := Sniff([]byte(tc.head))
		require.Equal(t, tc.format, format, name)
		require.Equal(t, tc.decided, decided, name)
	}
}

func TestStrip_RefusesFormatsItDoesNotRewrite(t *testing.T) {
	_, err := Strip(FormatNone, []byte("GIF89a"))
	require.Error(t, err)
}

func TestFormat_ContentTypeMatchesTheRewrittenBytes(t *testing.T) {
	require.Equal(t, "image/jpeg", FormatJPEG.ContentType())
	require.Equal(t, "image/png", FormatPNG.ContentType())
	require.Equal(t, "image/webp", FormatWebP.ContentType())
	require.Empty(t, FormatNone.ContentType())
}
