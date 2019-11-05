package sqlgen

import (
	"fmt"
)

// ErrorWithQuery is an error wrapper that includes
// the clause and arguments of a sqlgen query.
type ErrorWithQuery struct {
	err    error         // the wrapped error
	clause string        // the string clause of the query
	args   []interface{} // the args of the query

}

func (e *ErrorWithQuery) Error() string {
	if e.err == nil {
		return ""
	}
	return fmt.Sprintf("Error: %s; query clause: '%s'; query args: '%v'\n", e.err.Error(), e.clause, e.args)
}

// Unwrap returns the error without the clause
// or arguments.
func (e *ErrorWithQuery) Unwrap() error {
	return e.err
}

// Reason returns a human-readable error message of the clause and args
func (e *ErrorWithQuery) Reason() string {
	return fmt.Sprintf("Error in query clause: '%s'; query args: '%v'", e.clause, e.args)
}
