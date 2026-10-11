//go:build !windows

package tools

import (
	"context"
	"os/exec"
	"syscall"
)

// newLeafCommand builds the POSIX shell command that executes a leaf's
// verification/shell tool input.
func newLeafCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, "sh", "-c", command), nil
}

// configureLeafCommand puts the shell in its own process group so a
// timeout can signal the whole group, including background children.
func configureLeafCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killLeafProcessTree force-kills every process in the shell's group.
// A negative pid targets the process group led by the shell. ESRCH
// (empty group / already reaped) is expected and ignored.
func killLeafProcessTree(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
