package interfaceownership_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"go-check-it/internal/practices/interfaceownership"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, interfaceownership.Analyzer, "a")
}
