// Package practices runs a curated set of go/analysis checks that encode
// Go best practices (from Effective Go, the Go Wiki Code Review Comments,
// and the Google Go Style Guide) which are not already covered by go-check-it's
// CRAP score. Findings carry a structured explanation intended for both
// human readers and LLM agents acting on the report.
package practices

// Description is a structured, LLM-oriented explanation attached to every
// finding of a rule. Summary/Why/Fix are deliberately short: an agent (or a
// person) reading a finding should be able to act on it without further
// research, though DocURL is provided for the official source.
type Description struct {
	// Summary is a one-line, human-readable name for the rule.
	Summary string
	// Why explains the risk or cost of the pattern being flagged.
	Why string
	// Fix is a concrete, actionable instruction for resolving the finding.
	Fix string
	// DocURL points at the official guidance the rule is based on.
	DocURL string
}
