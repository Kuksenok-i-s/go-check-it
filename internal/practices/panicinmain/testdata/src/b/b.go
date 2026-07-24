// Package b is not package main, so panics here are out of scope for this rule.
package b

func Helper() {
	panic("not flagged: only package main is checked")
}
