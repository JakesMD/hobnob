package a

import "testing"

func TestCompliant(t *testing.T) {
	// given a compliant test, when linted, then nothing is reported (why: the
	// happy path must stay quiet)
	// Arrange
	x := 1

	// Act
	y := x + 1

	// Assert
	if y != 2 {
		t.Fatal(y)
	}
}

// given the comment as a doc comment, when linted, then it counts (why: the
// convention may sit above the func too)
func TestDocComment(t *testing.T) {
	// Arrange
	// Act
	// Assert
}

func TestTableDriven(t *testing.T) {
	// given markers inside t.Run, when linted, then they count (why: table
	// tests arrange per case)
	for _, c := range []int{1} {
		t.Run("", func(t *testing.T) {
			// Arrange
			_ = c
			// Act
			// Assert
		})
	}
}

func TestNothingToArrange(t *testing.T) {
	// given no setup, when linted, then // Arrange may be left out (why:
	// an empty Arrange block is noise)

	// Act
	y := 1

	// Assert
	_ = y
}

func TestArrangeLeftOutWithSetup(t *testing.T) { // want `TestArrangeLeftOutWithSetup: missing // Arrange`
	// given setup before // Act, when linted, then // Arrange is required
	// (why: setup must be labelled)
	x := 1

	// Act
	y := x

	// Assert
	_ = y
}

func TestNoGWT(t *testing.T) { // want `TestNoGWT: no given/when/then comment`
	// Arrange
	// Act
	// Assert
}

func TestNoWhy(t *testing.T) {
	// given no why, when linted, then reported // want `TestNoWhy: given/when/then comment has no .why: …. clause`
	// Arrange
	// Act
	// Assert
}

func TestNoMarkers(t *testing.T) { // want `TestNoMarkers: missing // Arrange, // Act, // Assert`
	// given no markers, when linted, then reported (why: structure is required)
}

func TestMarkerWithSuffix(t *testing.T) { // want `TestMarkerWithSuffix: missing // Arrange`
	// given a decorated marker, when linted, then it does not count (why:
	// markers are exact)
	// Arrange (inputs)
	x := 1
	// Act
	_ = x
	// Assert
}

func TestOutOfOrder(t *testing.T) { // want `TestOutOfOrder: // Arrange, // Act, // Assert must come in that order`
	// given markers out of order, when linted, then reported (why: the
	// blocks read top to bottom)
	// Act
	// Arrange
	// Assert
}

func TestMain(m *testing.M) {}

func Testlowercase(t *testing.T) {}

func helper(t *testing.T) {}

func BenchmarkX(b *testing.B) {}
