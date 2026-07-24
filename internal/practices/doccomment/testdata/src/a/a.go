package a

// wrong start.
func BadStart() {} // want `doc comment for BadStart should start with "BadStart"`

// BadEnd has no trailing period
func BadEnd() {} // want `doc comment for BadEnd should end with a period`

// Good does the right thing.
func Good() {}

func noDoc() {}

// Thing is a well documented type.
type Thing struct{}

// Exit codes returned by the CLI.
const (
	ExitOK    = 0
	ExitError = 1
)
