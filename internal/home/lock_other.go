//go:build !unix

package home

import (
	"errors"
	"os"
)

var errNoLocks = errors.New("file locks are not supported here")

func tryLockShared(*os.File) (bool, error) { return false, errNoLocks }

func lockExclusive(*os.File) error { return errNoLocks }

// DBIdentity is always 0 where file identities aren't available.
func (h *Home) DBIdentity() uint64 { return 0 }
