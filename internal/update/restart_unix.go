//go:build unix

package update

import (
	"os"
	"syscall"
)

// Restart replaces this process with the program at exe: same arguments, same process,
// so the port, the data folder and a running service stay. It returns only on failure.
func Restart(exe string) error {
	return syscall.Exec(exe, append([]string{exe}, RestartArgs(os.Args[1:])...), os.Environ())
}
