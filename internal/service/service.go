// Package service runs the board in the background, started at login: a LaunchAgent on
// macOS, a systemd user unit on Linux (a Raspberry Pi, an old laptop).
package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/azmainadel/kan-do/internal/home"
)

const (
	label = "com.github.azmainadel.kan-do"
	unit  = "kan-do.service"
)

// Install installs and starts the service, replacing an older one.
func Install(h *home.Home) error {
	exe, err := os.Executable()
	if err != nil {
		return errors.New("cannot find the kan-do program")
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(h, exe)
	case "linux":
		return installSystemd(h, exe)
	}
	return errors.New("kan-do service supports macOS and Linux; start `kan-do --no-open` with your own service manager")
}

// Uninstall stops and removes the service.
func Uninstall() error {
	switch runtime.GOOS {
	case "darwin":
		return uninstallLaunchd()
	case "linux":
		return uninstallSystemd()
	}
	return errors.New("kan-do service supports macOS and Linux")
}

// Restart restarts the service, if it's installed, so it runs a newly installed kan-do.
// It reports false when there's none.
func Restart() (bool, error) {
	switch runtime.GOOS {
	case "darwin":
		if path, err := plistPath(); err != nil || !exists(path) {
			return false, err
		}
		return true, run("launchctl", "kickstart", "-k", launchdDomain()+"/"+label)
	case "linux":
		if path, err := unitPath(); err != nil || !exists(path) {
			return false, err
		}
		return true, run("systemctl", "--user", "restart", unit)
	}
	return false, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// run runs a command and fails with its own error output.
func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("cannot run %s", name)
	}
	if err != nil {
		return fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

// macOS

func plistPath() (string, error) {
	h, err := os.UserHomeDir()
	return filepath.Join(h, "Library/LaunchAgents", label+".plist"), err
}

func launchdDomain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

var xmlEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func plist(exe, dir, log string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>--no-open</string>
        <string>--home</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%[4]s</string>
</dict>
</plist>
`, label, xmlEscape.Replace(exe), xmlEscape.Replace(dir), xmlEscape.Replace(log))
}

func installLaunchd(h *home.Home, exe string) error {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	log := filepath.Join(userHome, "Library/Logs/kan-do.log")
	path, _ := plistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	domain := launchdDomain()
	// Replace an older install so a changed path or data folder takes effect.
	exec.Command("launchctl", "bootout", domain+"/"+label).Run()
	if err := os.WriteFile(path, []byte(plist(exe, h.Dir, log)), 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	if err := run("launchctl", "bootstrap", domain, path); err != nil {
		return err
	}
	fmt.Printf("kan-do now starts at login and is running at http://localhost:%d\nLogs: %s\n", h.Config.Port, log)
	return nil
}

func uninstallLaunchd() error {
	path, err := plistPath()
	if err != nil {
		return err
	}
	exec.Command("launchctl", "bootout", launchdDomain()+"/"+label).Run()
	if !exists(path) {
		fmt.Println("The kan-do LaunchAgent was not installed.")
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("cannot remove %s: %w", path, err)
	}
	fmt.Println("Removed the kan-do LaunchAgent; the board no longer starts at login.")
	return nil
}

// Linux

func unitPath() (string, error) {
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		config = filepath.Join(h, ".config")
	}
	return filepath.Join(config, "systemd/user", unit), nil
}

// systemdQuote quotes one argument for a systemd ExecStart= line.
func systemdQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(s) + `"`
}

func unitFile(exe, dir string) string {
	return fmt.Sprintf(`[Unit]
Description=kan-do board
After=network.target

[Service]
ExecStart=%s --no-open --home %s
Restart=on-failure

[Install]
WantedBy=default.target
`, systemdQuote(exe), systemdQuote(dir))
}

func installSystemd(h *home.Home, exe string) error {
	path, err := unitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(unitFile(exe, h.Dir)), 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", unit}, {"restart", unit}} {
		// Restart rather than start, so a changed path or data folder takes effect.
		if err := run("systemctl", append([]string{"--user"}, args...)...); err != nil {
			return err
		}
	}
	fmt.Printf("kan-do now starts at login and is running at http://localhost:%d\n"+
		"Logs: journalctl --user -u kan-do\n"+
		"To start it at boot without logging in (a board with no screen): sudo loginctl enable-linger $USER\n", h.Config.Port)
	return nil
}

func uninstallSystemd() error {
	path, err := unitPath()
	if err != nil {
		return err
	}
	exec.Command("systemctl", "--user", "disable", "--now", unit).Run()
	if !exists(path) {
		fmt.Println("The kan-do service was not installed.")
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("cannot remove %s: %w", path, err)
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	fmt.Println("Removed the kan-do service; the board no longer starts at login.")
	return nil
}
