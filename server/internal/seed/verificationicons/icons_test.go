package verificationicons

import (
	"bytes"
	"image"
	_ "golang.org/x/image/webp"
	"testing"
)

// TestBundledIconsAreRenderableWebP pins the one failure mode that is invisible
// at runtime: a badge document whose body is not a valid custom-emoji image
// decodes to nothing in the client, so the peer looks unverified even though the
// catalogue and the mark agree it is verified.
func TestBundledIconsAreRenderableWebP(t *testing.T) {
	if len(Icons) == 0 {
		t.Fatal("no verification icons are bundled")
	}
	seenID := make(map[int64]string, len(Icons))
	seenSlug := make(map[string]string, len(Icons))
	for _, icon := range Icons {
		if icon.DocumentID <= 0 {
			t.Fatalf("%s: document id = %d", icon.Name, icon.DocumentID)
		}
		if previous, ok := seenID[icon.DocumentID]; ok {
			t.Fatalf("%s and %s share document id %d", previous, icon.Name, icon.DocumentID)
		}
		seenID[icon.DocumentID] = icon.Name
		if previous, ok := seenSlug[icon.Slug]; ok {
			t.Fatalf("%s and %s share slug %q", previous, icon.Name, icon.Slug)
		}
		seenSlug[icon.Slug] = icon.Name

		body, err := FS.ReadFile(icon.File)
		if err != nil {
			t.Fatalf("%s: read %s: %v", icon.Name, icon.File, err)
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("%s: decode %s: %v", icon.Name, icon.File, err)
		}
		if format != "webp" {
			t.Fatalf("%s: format = %q, want webp", icon.Name, format)
		}
		if config.Width != 100 || config.Height != 100 {
			t.Fatalf("%s: %dx%d, want 100x100 custom emoji body", icon.Name, config.Width, config.Height)
		}
	}
}
