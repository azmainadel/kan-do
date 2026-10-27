//go:build !unix

package update

import "errors"

// Restart can't replace a running program here; the person restarts it.
func Restart(string) error { return errors.New("restart kan-do to finish the update") }
