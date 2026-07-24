# Go practice rule catalog

These are the ten AST-based practice checks built into go-check-it. The JSON report
contains the applicable rationale, fix, and source; use this catalog when the
tool is unavailable or when reviewing a heuristic result.

## panicinmain

- **Flags:** `panic()` reachable from package main.
- **Fix:** return an error to `main`, log useful context, and exit non-zero.
- **Source:** https://go.dev/doc/effective_go#errors

## errorstrings

- **Flags:** `errors.New` or `fmt.Errorf` messages that start with a capital
  letter or end with punctuation.
- **Fix:** start lowercase except for initialisms and omit trailing punctuation.
- **Source:** https://go.dev/wiki/CodeReviewComments#error-strings

## doccomment

- **Flags:** exported declaration comments that do not start with the declared
  name or end with a period.
- **Fix:** write a complete sentence beginning with the declaration name.
- **Source:** https://go.dev/doc/effective_go#commentary

## contextprop

- **Flags:** context-free process and HTTP APIs when context-aware variants are
  available.
- **Fix:** use `exec.CommandContext`, or create an HTTP request with context and
  send it through a client.
- **Source:** https://go.dev/blog/context

## interfaceownership

- **Flags:** an interface declared beside its only implementation with no local
  consumer.
- **Fix:** define the interface in the consuming package, or remove it if no
  abstraction is needed.
- **Source:** https://go.dev/wiki/CodeReviewComments#interfaces

## receivername

- **Flags:** methods on one type using inconsistent receiver names.
- **Fix:** use one short receiver name consistently for that type.
- **Source:** https://go.dev/wiki/CodeReviewComments#receiver-names

## nakedreturn

- **Flags:** naked returns in functions longer than roughly 20 lines with named
  results.
- **Fix:** return the values explicitly.
- **Source:** https://go.dev/wiki/CodeReviewComments#named-result-parameters

## initialism

- **Flags:** common initialisms cased as normal words, such as `Id`, `Url`, or
  `Http`.
- **Fix:** use `ID`, `URL`, `HTTP`, `API`, `JSON`, and similar conventional
  casing.
- **Source:** https://go.dev/wiki/CodeReviewComments#initialisms

## underscorename

- **Flags:** package-level identifiers containing underscores, except test
  function names.
- **Fix:** use MixedCaps or mixedCaps.
- **Source:** https://go.dev/doc/effective_go#mixed-caps

## goroutinelifetime

- **Flags:** an inline goroutine with no visible context, select, channel, or
  wait-group exit signal.
- **Fix:** add an explicit lifetime mechanism or document why the goroutine is
  intentionally process-long.
- **Confidence:** heuristic score from 0 to 10; only scores above the neutral
  midpoint are reported.
- **Source:** https://go.dev/blog/context
