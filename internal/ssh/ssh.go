package ssh

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func Run(host, command string) (string, int, error) {
	if strings.TrimSpace(host) == "" {
		return "", 0, fmt.Errorf("ssh host is required")
	}
	cmd := exec.Command("ssh", "-T", host, "bash", "-lc", command)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			return buf.String(), 0, err
		}
	}
	return buf.String(), exitCode, err
}
