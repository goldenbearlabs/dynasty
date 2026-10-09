// Package problem marks errors that are the user's to fix: a broken rule or
// invalid input. Their text is shown to the user as written; any other error
// is logged and reported as an internal failure.
package problem

import "fmt"

type Error string

func (e Error) Error() string { return string(e) }

func New(format string, args ...any) Error {
	return Error(fmt.Sprintf(format, args...))
}
