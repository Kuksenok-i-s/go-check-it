// Package goroutinelifetime flags "go func() { ... }()" launches whose exit
// condition isn't visible from syntax alone (no context, no channel, no
// WaitGroup, no obvious break out of an infinite loop) — a common source of
// goroutine leaks (Go blog, "Never start a goroutine without knowing how it
// will stop").
//
// This cannot be verified soundly without running the program, so each
// finding self-reports a 0-10 confidence: how likely, from syntactic signals
// alone, that this goroutine's lifetime is genuinely unclear. Only launches
// scoring above the neutral midpoint are reported, to keep the signal usable
// rather than spamming every goroutine in the codebase. It only inspects
// inline closures (go func(){...}()); goroutines launched by calling a named
// function are out of scope because their body may live in another package.
package goroutinelifetime

import (
	"go/ast"
	"go/token"
	"strconv"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports goroutine launches with an unclear exit condition.
var Analyzer = &analysis.Analyzer{
	Name: "goroutinelifetime",
	Doc:  "reports go func(){...}() launches with no visible exit condition, confidence-scored 0-10",
	Run:  run,
}

// reportThreshold is the minimum risk score (see scoreBody) worth surfacing.
const reportThreshold = 6

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			goStmt, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			lit, ok := goStmt.Call.Fun.(*ast.FuncLit)
			if !ok {
				return true // call to a named function: out of scope, see package doc
			}
			score := scoreBody(lit.Body)
			if score < reportThreshold {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos: goStmt.Pos(),
				Message: "goroutine lifetime unclear: no context/select/channel/WaitGroup signal found for when " +
					"it exits; document or make the exit condition explicit",
				Category: confidenceCategory(score),
			})
			return true
		})
	}
	return nil, nil
}

func confidenceCategory(score int) string {
	return "confidence:" + strconv.Itoa(score)
}

// bodySignals are the syntactic cues scoreBody uses to estimate risk.
type bodySignals struct {
	hasSelect, hasDoneCall, hasChanOp, hasReturn, hasBareInfiniteLoop bool
}

// scoreBody estimates, from 0 (clearly fine) to 10 (clearly unclear), how
// likely a goroutine body's lifetime is unclear from syntax alone.
func scoreBody(body *ast.BlockStmt) int {
	return scoreFromSignals(scanBodySignals(body))
}

func scanBodySignals(body *ast.BlockStmt) bodySignals {
	var s bodySignals
	ast.Inspect(body, func(n ast.Node) bool {
		noteBodyNode(n, &s)
		return true
	})
	return s
}

func noteBodyNode(n ast.Node, s *bodySignals) {
	switch v := n.(type) {
	case *ast.SelectStmt:
		s.hasSelect = true
	case *ast.SendStmt:
		s.hasChanOp = true
	case *ast.UnaryExpr:
		noteChanRecv(v, s)
	case *ast.ReturnStmt:
		s.hasReturn = true
	case *ast.CallExpr:
		noteDoneCall(v, s)
	case *ast.ForStmt:
		noteBareInfiniteLoop(v, s)
	}
}

func noteChanRecv(v *ast.UnaryExpr, s *bodySignals) {
	if v.Op == token.ARROW {
		s.hasChanOp = true
	}
}

func noteDoneCall(v *ast.CallExpr, s *bodySignals) {
	sel, ok := v.Fun.(*ast.SelectorExpr)
	if ok && sel.Sel.Name == "Done" {
		s.hasDoneCall = true
	}
}

func noteBareInfiniteLoop(v *ast.ForStmt, s *bodySignals) {
	if v.Cond == nil && v.Init == nil && v.Post == nil && !loopHasExit(v.Body) {
		s.hasBareInfiniteLoop = true
	}
}

func scoreFromSignals(s bodySignals) int {
	return clampScore(5 - signalRelief(s) + signalRisk(s))
}

func signalRelief(s bodySignals) int {
	n := 0
	if s.hasSelect {
		n += 3
	}
	if s.hasDoneCall {
		n += 3
	}
	if s.hasChanOp {
		n += 2
	}
	return n
}

func signalRisk(s bodySignals) int {
	n := 0
	if s.hasBareInfiniteLoop {
		n += 4
	}
	if noExitSignal(s) {
		n += 2
	}
	return n
}

func noExitSignal(s bodySignals) bool {
	return !s.hasReturn && !s.hasSelect && !s.hasChanOp && !s.hasDoneCall
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 10 {
		return 10
	}
	return score
}

// loopHasExit reports whether a for-loop body contains a way out: break,
// return, or select (select cases are commonly used with a ctx.Done() case).
func loopHasExit(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if nodeHasLoopExit(n) {
			found = true
		}
		return true
	})
	return found
}

func nodeHasLoopExit(n ast.Node) bool {
	switch v := n.(type) {
	case *ast.BranchStmt:
		return v.Tok == token.BREAK
	case *ast.ReturnStmt, *ast.SelectStmt:
		return true
	}
	return false
}
