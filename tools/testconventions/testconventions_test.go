package testconventions_test

import (
	"testing"

	"github.com/JakesMD/hobnob/tools/testconventions"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	// given a package of compliant and non-compliant tests, when analyzed,
	// then exactly the // want lines are reported (why: the linter must
	// catch every broken convention without flagging helpers, benchmarks or
	// TestMain)
	// Arrange
	dir := analysistest.TestData()

	// Act
	results := analysistest.Run(t, dir, testconventions.Analyzer, "a")

	// Assert
	for _, result := range results {
		if result.Err != nil {
			t.Fatalf("analysis failed: %v", result.Err)
		}
	}
}
