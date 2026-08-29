package shellcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const Timeout = 5 * time.Second

type cappedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (w *cappedBuffer) Write(data []byte) (int, error) {
	originalLen := len(data)
	if w.limit > 0 {
		if len(data) > w.limit {
			data = data[:w.limit]
		}
		_, _ = w.buffer.Write(data)
		w.limit -= len(data)
	}
	return originalLen, nil
}

func Run(command string, rawLevel int) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("the shell command is empty")
	}
	if rawLevel < 0 {
		rawLevel = 0
	}
	if rawLevel > 255 {
		rawLevel = 255
	}
	level := (rawLevel*100 + 127) / 255
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Env = append(os.Environ(),
		"LEVEL="+strconv.Itoa(level),
		"RAW_LEVEL="+strconv.Itoa(rawLevel),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stderr := &cappedBuffer{limit: 4096}
	cmd.Stdout = &cappedBuffer{limit: 4096}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start shell: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(Timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err == nil {
			return nil
		}
		message := strings.TrimSpace(stderr.buffer.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("shell command: %s", message)
	case <-timer.C:
		// Kill the complete group, not only /bin/sh, so a child cannot outlive the
		// timeout and continue consuming resources in the background.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return fmt.Errorf("shell command exceeded %s", Timeout)
	}
}
