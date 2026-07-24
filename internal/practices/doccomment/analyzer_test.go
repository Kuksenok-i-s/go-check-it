package doccomment_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"go-check-it/internal/practices/doccomment"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, doccomment.Analyzer, "a")
}
