package queue

import (
	"os/exec"
	"testing"
	"time"
)

func TestInterruptCmdStopsSleep(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	prepareCmd(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	interruptCmd(cmd)
	err := cmd.Wait()
	if time.Since(start) > 3*time.Second {
		t.Fatalf("process took %s to exit after SIGTERM", time.Since(start))
	}
	if err == nil {
		t.Fatal("expected sleep to exit with error after SIGTERM")
	}
}

func TestKillCmdStopsSleep(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	prepareCmd(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	killCmd(cmd)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("SIGKILL did not stop process")
	}
}

func TestPrepareCmdSetsPgid(t *testing.T) {
	cmd := exec.Command("true")
	prepareCmd(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("expected Setpgid")
	}
}

func TestInterruptNilCmd(t *testing.T) {
	interruptCmd(nil)
	killCmd(nil)
	interruptCmd(&exec.Cmd{})
	killCmd(&exec.Cmd{})
}
