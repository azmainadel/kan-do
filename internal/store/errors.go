package store

import (
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Kind says what went wrong, so callers can map an error to an HTTP status.
type Kind int

const (
	// Invalid input, like an empty title or an unknown status.
	Invalid Kind = iota + 1
	// NotFound is a project, task, comment or image that doesn't exist.
	NotFound
	// Busy means another process holds the data folder for longer than we wait, like a long sync.
	Busy
)

// Error is a failure the person or agent can act on; its message says how.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// KindOf returns the Kind of err, or 0 for anything else, like a database failure.
func KindOf(err error) Kind {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Kind
	}
	return 0
}

func invalid(format string, a ...any) error {
	return &Error{Kind: Invalid, Msg: fmt.Sprintf(format, a...)}
}

func notFound(format string, a ...any) error {
	return &Error{Kind: NotFound, Msg: fmt.Sprintf(format, a...)}
}

// BusyError is the error for a data folder held by sync.
func BusyError(msg string) error {
	return &Error{Kind: Busy, Msg: msg}
}

// dbErr labels a database failure; nil and our own errors pass through.
func dbErr(err error) error {
	if err == nil || KindOf(err) != 0 {
		return err
	}
	return fmt.Errorf("database error: %w", err)
}

func isConstraint(err error) bool {
	e, ok := errors.AsType[*sqlite.Error](err)
	return ok && e.Code()&0xff == sqlite3.SQLITE_CONSTRAINT
}
