//go:build unix

package home

import (
	"errors"
	"os"
	"syscall"
)

// tryLockShared takes a shared lock without waiting. It reports false when sync holds
// the exclusive one, and an error when the filesystem has no locks.
func tryLockShared(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func lockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// DBIdentity changes when the database file itself is replaced, as sync does when it
// pulls, so the board reloads even if the pulled change counter equals the old one.
func (h *Home) DBIdentity() uint64 {
	info, err := os.Stat(h.DBPath())
	if err != nil {
		return 0
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Ino)
	}
	return 0
}
