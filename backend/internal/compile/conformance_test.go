package compile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestConformanceGolden fails when `wbc conformance` would change contract/conformance/compiled/.
func TestConformanceGolden(t *testing.T) {
	c, err := LoadContract(contractDir)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Conformance(c, filepath.Join(contractDir, "conformance"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range ConformanceFiles {
		want, err := os.ReadFile(filepath.Join(contractDir, "conformance", "compiled", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out[name], want) {
			t.Errorf("compiled/%s.json is stale: run `go run ./cmd/wbc conformance -contract ../contract`", name)
		}
	}
}

// TestCompiledScreensGolden fails when `wbc conformance` would change contract/conformance/compiled-screens/: the SDK
// tests run those contract screens' handlers from the committed wire documents.
func TestCompiledScreensGolden(t *testing.T) {
	c, err := LoadContract(contractDir)
	if err != nil {
		t.Fatal(err)
	}
	out, err := CompiledScreens(c, contractDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ConformanceScreens {
		want, err := os.ReadFile(filepath.Join(contractDir, "conformance", "compiled-screens", id+".json"))
		if err != nil || !bytes.Equal(out[id], want) {
			t.Errorf("compiled-screens/%s.json is stale: run `go run ./cmd/wbc conformance -contract ../contract`", id)
		}
	}
}
