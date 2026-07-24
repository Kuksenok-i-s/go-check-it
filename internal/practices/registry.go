package practices

import (
	"golang.org/x/tools/go/analysis"

	"go-check-it/internal/practices/contextprop"
	"go-check-it/internal/practices/doccomment"
	"go-check-it/internal/practices/errorstrings"
	"go-check-it/internal/practices/goroutinelifetime"
	"go-check-it/internal/practices/initialism"
	"go-check-it/internal/practices/interfaceownership"
	"go-check-it/internal/practices/nakedreturn"
	"go-check-it/internal/practices/panicinmain"
	"go-check-it/internal/practices/receivername"
	"go-check-it/internal/practices/underscorename"
)

type ruleEntry struct {
	Analyzer    *analysis.Analyzer
	Description Description
}

// rules is the full set of practice checks go-check-it runs. Each entry pairs a
// go/analysis Analyzer with the structured explanation attached to its
// findings.
var rules = []ruleEntry{
	{
		Analyzer: panicinmain.Analyzer,
		Description: Description{
			Summary: "Don't panic in package main",
			Why:     "A command that panics prints a raw stack trace to the user instead of a clear error, and skips any cleanup deferred by the caller.",
			Fix:     "Return an error up to main, log it with context, and os.Exit(1) instead of calling panic.",
			DocURL:  "https://go.dev/doc/effective_go#errors",
		},
	},
	{
		Analyzer: errorstrings.Analyzer,
		Description: Description{
			Summary: "Error strings should be lowercase and unpunctuated",
			Why:     "Error strings are usually wrapped and printed alongside other context (e.g. \"connect: %w\"); a capital letter or trailing period looks wrong once embedded.",
			Fix:     "Rewrite the message to start lowercase (except initialisms) and drop any trailing period.",
			DocURL:  "https://github.com/golang/go/wiki/CodeReviewComments#error-strings",
		},
	},
	{
		Analyzer: doccomment.Analyzer,
		Description: Description{
			Summary: "Doc comments should start with the name and end with a period",
			Why:     "Godoc and IDE hovers extract the first sentence verbatim; comments that don't start with the identifier or end with a period read as broken fragments.",
			Fix:     "Start the comment with the declared name (\"Foo does ...\") and end the sentence with a period.",
			DocURL:  "https://go.dev/doc/effective_go#commentary",
		},
	},
	{
		Analyzer: contextprop.Analyzer,
		Description: Description{
			Summary: "Propagate context to subprocesses and HTTP calls",
			Why:     "Without a context, callers cannot cancel or time out the call; a hung subprocess or slow server holds resources until it finishes on its own.",
			Fix:     "Use the *Context sibling (exec.CommandContext, http.NewRequestWithContext) and thread a context.Context through.",
			DocURL:  "https://go.dev/blog/context",
		},
	},
	{
		Analyzer: interfaceownership.Analyzer,
		Description: Description{
			Summary: "Define interfaces where they're consumed, not where they're implemented",
			Why:     "An interface declared beside its only implementation, with no local consumer, usually exists for the producer's convenience (or for a mock) rather than because any caller needs the abstraction.",
			Fix:     "Move the interface into the package that calls through it (defined at the point of use), or delete it if nothing actually depends on the abstraction.",
			DocURL:  "https://github.com/golang/go/wiki/CodeReviewComments#interfaces",
		},
	},
	{
		Analyzer: receivername.Analyzer,
		Description: Description{
			Summary: "Use one consistent receiver name per type",
			Why:     "Different receiver names across a type's methods (e.g. w vs widget) read as if the methods operate on different things.",
			Fix:     "Pick one short abbreviation for the type and use it as the receiver name on every method.",
			DocURL:  "https://github.com/golang/go/wiki/CodeReviewComments#receiver-names",
		},
	},
	{
		Analyzer: nakedreturn.Analyzer,
		Description: Description{
			Summary: "Avoid naked returns in long functions",
			Why:     "In a short function a naked return is obvious; once a function is long enough that the named results have scrolled off screen, a bare 'return' hides what's actually returned.",
			Fix:     "Return the values explicitly (return n, err) once the function body is more than a couple dozen lines.",
			DocURL:  "https://github.com/golang/go/wiki/CodeReviewComments#named-result-parameters",
		},
	},
	{
		Analyzer: initialism.Analyzer,
		Description: Description{
			Summary: "Keep common initialisms fully uppercase",
			Why:     "Id, Url, Http, Api etc. read as odd half-measures; Go convention treats initialisms as a single word and keeps them fully capitalized.",
			Fix:     "Rename to the conventional casing: ID, URL, HTTP, API, JSON, and so on.",
			DocURL:  "https://github.com/golang/go/wiki/CodeReviewComments#initialisms",
		},
	},
	{
		Analyzer: underscorename.Analyzer,
		Description: Description{
			Summary: "Use MixedCaps, not snake_case",
			Why:     "Underscored names stand out as non-idiomatic in Go and are inconsistent with the rest of the standard library and ecosystem.",
			Fix:     "Rename using MixedCaps (or mixedCaps for unexported names).",
			DocURL:  "https://go.dev/doc/effective_go#mixed-caps",
		},
	},
	{
		Analyzer: goroutinelifetime.Analyzer,
		Description: Description{
			Summary: "Every goroutine should have a visible way to stop",
			Why:     "A goroutine launched with no context, channel, select, or WaitGroup signal in sight is a common way to leak goroutines that never exit.",
			Fix:     "Wire in a context (select on ctx.Done()), a done channel, or a sync.WaitGroup, or document why the goroutine is expected to run forever.",
			DocURL:  "https://go.dev/blog/context",
		},
	},
}
