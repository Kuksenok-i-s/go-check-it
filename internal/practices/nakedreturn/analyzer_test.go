package nakedreturn_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"go-check-it/internal/practices/nakedreturn"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, nakedreturn.Analyzer, "a")
}
