package eval

import "testing"

func TestCopyVars(t *testing.T) {
	// given source map, when CopyVars called and dst mutated, then source unchanged (why: scope isolation depends on deep copy semantics)
	// Arrange
	src := map[string]string{"A": "1", "B": "2"}

	// Act
	dst := CopyVars(src)
	dst["A"] = "changed"

	// Assert
	if src["A"] != "1" {
		t.Errorf("CopyVars mutated source: src[A]=%q", src["A"])
	}
	if dst["B"] != "2" {
		t.Errorf("CopyVars missing key: dst[B]=%q", dst["B"])
	}
}
