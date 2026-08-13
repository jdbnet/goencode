package queue

import (
	"os/exec"
	"syscall"
	"time"
)

const killEscalateAfter = 5 * time.Second

func prepareCmd(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func interruptCmd(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
}

func killCmd(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}

func escalateKill(cmd *exec.Cmd, stillActive func(*exec.Cmd) bool) {
	if cmd == nil {
		return
	}
	timer := time.NewTimer(killEscalateAfter)
	defer timer.Stop()
	<-timer.C
	if stillActive(cmd) {
		killCmd(cmd)
	}
}
