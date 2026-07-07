package xcstrings

import (
	"bytes"
	"os"
	"testing"
)

// TestRoundTripExternalFile verifies the byte-identical round trip against a
// real-world catalog outside the repo. Set XLOCAL_ROUNDTRIP_FILE to the path
// of an Xcode-authored .xcstrings file to enable it.
func TestRoundTripExternalFile(t *testing.T) {
	path := os.Getenv("XLOCAL_ROUNDTRIP_FILE")
	if path == "" {
		t.Skip("XLOCAL_ROUNDTRIP_FILE not set")
	}

	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	file, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Marshal(file)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(original, out) {
		firstDiff := 0
		for firstDiff < len(original) && firstDiff < len(out) && original[firstDiff] == out[firstDiff] {
			firstDiff++
		}
		lo := firstDiff - 100
		if lo < 0 {
			lo = 0
		}
		hiA, hiB := firstDiff+100, firstDiff+100
		if hiA > len(original) {
			hiA = len(original)
		}
		if hiB > len(out) {
			hiB = len(out)
		}
		t.Errorf("round trip differs at byte %d\n--- original ---\n%s\n--- marshaled ---\n%s",
			firstDiff, original[lo:hiA], out[lo:hiB])
	}
}
