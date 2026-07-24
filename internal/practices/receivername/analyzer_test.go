package receivername_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"go-check-it/internal/practices/receivername"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, receivername.Analyzer, "a")
}
