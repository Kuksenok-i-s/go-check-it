package a

import (
	"errors"
	"fmt"
	"strings"
)

var errBad = errors.New("Something failed") // want `error string should not be capitalized`

var errBad2 = fmt.Errorf("write failed.") // want `error string should not end with punctuation`

var errGood = errors.New("something failed")

var errInitialism = errors.New("HTTP request failed")

var errEmpty = errors.New("")

var errNonLiteral = errors.New(someMsg)

var errNotCtor = errors.Unwrap(nil)

var errOtherPkg = strings.TrimSpace("x")

var someMsg = "x"

func plain() error { return nil }

var _ = plain()
