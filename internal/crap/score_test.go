package crap

import "testing"

func f64(v float64) *float64 { return &v }

func TestCalculate_NilCoverageYieldsNil(t *testing.T) {
	if got := Calculate(10, nil); got != nil {
		t.Fatalf("expected nil, got %v", *got)
	}
}

func TestCalculate_FullCoverageEqualsComplexity(t *testing.T) {
	got := Calculate(15, f64(100))
	if got == nil {
		t.Fatal("expected non-nil score")
	}
	if *got != 15 {
		t.Fatalf("expected 15, got %v", *got)
	}
}

func TestCalculate_ZeroCoverage(t *testing.T) {
	got := Calculate(15, f64(0))
	want := 15.0*15.0 + 15.0
	if got == nil || *got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCalculate_PartialCoverage(t *testing.T) {
	got := Calculate(15, f64(50))
	want := 225.0*0.125 + 15.0
	if got == nil || diff(*got, want) > 1e-9 {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCalculate_LowComplexityLowCoverage(t *testing.T) {
	got := Calculate(1, f64(0))
	want := 1.0 + 1.0
	if got == nil || *got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func diff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
