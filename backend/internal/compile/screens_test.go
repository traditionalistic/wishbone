package compile

import (
	"os"
	"path/filepath"
	"testing"
)

// Every shipped screen compiles against the real contract (not only at server seed time).
func TestContractScreensCompile(t *testing.T) {
	c, err := LoadContract(contractDir)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(contractDir, "screens", "*.json"))
	if len(files) == 0 {
		t.Fatal("no contract screens found")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, diags := Screen(c, src, 1); len(diags) > 0 {
			t.Errorf("%s: %v", filepath.Base(f), diags)
		}
	}
}
