package eval

import "testing"

func TestCloneMap(t *testing.T) {
	// given source map, when CloneMap called and dst mutated, then source unchanged (why: scope isolation depends on deep copy semantics)
	// Arrange
	src := map[string]string{"A": "1", "B": "2"}

	// Act
	dst := CloneMap(src)
	dst["A"] = "changed"

	// Assert
	if src["A"] != "1" {
		t.Errorf("CloneMap mutated source: src[A]=%q", src["A"])
	}
	if dst["B"] != "2" {
		t.Errorf("CloneMap missing key: dst[B]=%q", dst["B"])
	}
}

func TestCloneMap_NilSourceReturnsNonNil(t *testing.T) {
	// given nil source map, when CloneMap called, then returns a non-nil empty map (why: callers write into the result unconditionally)
	// Act
	dst := CloneMap[string, string](nil)

	// Assert
	if dst == nil {
		t.Fatal("got nil, want non-nil empty map")
	}
	if len(dst) != 0 {
		t.Errorf("got %v, want empty", dst)
	}
}
