# Reducing CRAP: functions vs. packages

Read this when go-check-it reports high-CRAP functions and you need to decide
how to fix them.

## Rules

1. Prefer tests that add meaningful branch coverage over restructuring code.
   Never add superficial tests solely to raise coverage numbers.
2. If a function is genuinely multi-purpose, split it into unexported helper
   functions in the **same package** as the caller (a same-package
   `helpers.go` file is enough). This is the default move.
3. Never create a new package for a single extracted helper.
4. Only promote a helper to its own package when it is already reused by two
   or more otherwise-unrelated packages and forms a cohesive concept on its
   own. When in doubt, prefer one fewer package over one more.
5. Never grow a catch-all `utils`/`helpers` package by dumping unrelated
   functions into it just because they're small.

## Example

Bad — a new package for one helper used from a single call site:

```go
// billing/pricehelpers/pricehelpers.go
package pricehelpers

func RoundToCents(v float64) int64 { /* ... */ }
```

```go
// billing/invoice.go
import "example.com/app/billing/pricehelpers"

func Total(items []Item) int64 {
	return pricehelpers.RoundToCents(sum(items))
}
```

Good — same extraction, kept in the caller's package:

```go
// billing/invoice.go
package billing

func Total(items []Item) int64 {
	return roundToCents(sum(items))
}

func roundToCents(v float64) int64 { /* ... */ }
```

Good — promoted to a shared package only once genuinely needed by both
`billing` and `payroll`, with no other coupling between them:

```go
// internal/money/money.go
package money

func RoundToCents(v float64) int64 { /* ... */ }
```
