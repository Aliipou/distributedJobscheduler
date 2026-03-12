package worker

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// executeCommand runs a job command with the given timeout.
// Commands starting with "http://" or "https://" are treated as HTTP callbacks.
// All other commands are executed as shell commands.
func executeCommand(ctx context.Context, command, payload string, timeoutSec int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	if strings.HasPrefix(command, "http://") || strings.HasPrefix(command, "https://") {
		return executeHTTP(ctx, command, payload)
	}
	return executeShell(ctx, command)
}

func executeShell(ctx context.Context, command string) (string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	var cmd *exec.Cmd
	if len(parts) == 1 {
		cmd = exec.CommandContext(ctx, parts[0])
	} else {
		cmd = exec.CommandContext(ctx, parts[0], parts[1:]...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		combined := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return combined, fmt.Errorf("command failed: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func executeHTTP(ctx context.Context, url, payload string) (string, error) {
	var body *bytes.Reader
	if payload != "" {
		body = bytes.NewReader([]byte(payload))
	} else {
		body = bytes.NewReader([]byte("{}"))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)

	if resp.StatusCode >= 400 {
		return buf.String(), fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	return buf.String(), nil
}
