//go:build windows

package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

// createNoWindow prevents a console window from flashing for each leaf
// command launched from the GUI sidecar.
const createNoWindow = 0x08000000

// newLeafCommand locates a POSIX shell. divvy's verification dialect is
// shell-shaped (sh -c, pipelines, rm -rf, /dev/null), so on Windows the
// supported host is Git for Windows, whose sh.exe is on PATH when its
// "Git Bash" is installed. cmd.exe/PowerShell are deliberately not used:
// silently rewriting the command dialect would make leaves fail in
// confusing, non-portable ways.
func newLeafCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		return nil, fmt.Errorf(
			"shell tools require a POSIX sh on PATH; install Git for Windows so sh.exe is available: %w",
			err,
		)
	}
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	return cmd, nil
}

// configureLeafCommand hides the shell's console window. Process-tree
// cleanup uses taskkill /T rather than POSIX process groups.
func configureLeafCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// killLeafProcessTree force-kills the shell and every descendant
// (taskkill /T tracks the whole parent-child tree, /F terminates).
// Best effort: an already-exited shell or empty tree is not an error.
func killLeafProcessTree(pid int) error {
	kill := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	kill.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	_ = kill.Run()
	return nil
}
