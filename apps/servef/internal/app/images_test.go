package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/servef/pkg/contract"
)

func TestReadImageIsRootConfinedImagesOnlyAndBounded(t *testing.T) {
	application, root := newTestApp(t)
	png := []byte("\x89PNG\r\n\x1a\n")
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{"assets/a.png": png, "big.png": bytes.Repeat([]byte{1}, contract.MaxImageBytes+1)} {
		if err := os.WriteFile(filepath.Join(root, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	image, err := application.Documents.ReadImage(root, "assets/a.png")
	if err != nil || image.MediaType != "image/png" || !bytes.Equal(image.Content, png) {
		t.Fatalf("image = %q %q, %v", image.MediaType, image.Content, err)
	}
	cases := map[string]struct {
		project, path string
		kind          contract.ErrorKind
		message       string
	}{
		"text":    {root, "data.json", contract.KindNotFound, contract.ImageUnavailableMessage},
		"missing": {root, "gone.png", contract.KindNotFound, contract.ImageUnavailableMessage},
		"escape":  {root, "../a.png", contract.KindNotFound, contract.ImageUnavailableMessage},
		"project": {filepath.Join(root, "gone"), "assets/a.png", contract.KindNotFound, contract.ImageUnavailableMessage},
		"large":   {root, "big.png", contract.KindTooLarge, contract.ImageTooLargeMessage},
	}
	for name, test := range cases {
		_, err := application.Documents.ReadImage(test.project, test.path)
		var failure *Error
		if !errors.As(err, &failure) || failure.Kind != test.kind || failure.Message != test.message {
			t.Errorf("%s error = %v, want %s %q", name, err, test.kind, test.message)
		}
	}
}
