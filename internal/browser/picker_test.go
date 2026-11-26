package browser

import (
	"os"
	"path/filepath"
	"testing"

	"hobnob/internal/ask"
)

func TestPickerStart_DirDefaultFailingMatchOpensThatFolder(t *testing.T) {
	// given a directory default that exists but fails match:, when the page opens a dir picker, then it opens that folder with nothing picked
	// (why: the person lands where the default is)

	// Arrange
	root := t.TempDir()
	for _, dir := range []string{"x", "y"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	onlyX := func(abs string, _ bool) bool { return filepath.Base(abs) == "x" }
	view := newPickerView(ask.PathRules{Dir: true, Invoke: root, Defaults: []string{"y"}, Included: onlyX})

	// Act
	open, selected := view.start(false)

	// Assert
	if open != filepath.Join(root, "y") || len(selected) != 0 {
		t.Errorf("start() = %q, %q; want the y folder and nothing picked", open, selected)
	}
}
