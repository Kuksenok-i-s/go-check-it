package underscorename_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"go-check-it/internal/practices/underscorename"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, underscorename.Analyzer, "a")
}
