// Package crap implements the CRAP (Change Risk Anti-Patterns) score formula.
package crap

// Calculate returns the CRAP score for a function with the given cyclomatic
// complexity and coverage percentage (0..100). It returns nil when coverage
// is unknown, since CRAP is undefined without a coverage measurement.
//
// CRAP(m) = comp(m)^2 * (1 - cov(m)/100)^3 + comp(m).
func Calculate(complexity int, coveragePercent *float64) *float64 {
	if coveragePercent == nil {
		return nil
	}
	cc := float64(complexity)
	uncovered := 1.0 - (*coveragePercent / 100.0)
	score := (cc * cc * uncovered * uncovered * uncovered) + cc
	return &score
}
