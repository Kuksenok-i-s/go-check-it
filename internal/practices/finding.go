package practices

import "go/token"

// Finding is one reported violation of a practice rule, located in source
// and enriched with the rule's Description so downstream consumers (report
// formatters, JSON output, LLM agents) never need a side lookup table.
type Finding struct {
	Rule        string
	Message     string
	Position    token.Position
	Description Description
	// Confidence is a 0-10 self-reported certainty for heuristic rules that
	// cannot be verified soundly from syntax alone (10 = certain). Rules that
	// are exact leave this at 10.
	Confidence int
}
