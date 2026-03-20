package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExecuteShell_Success(t *testing.T) {
	output, err := executeCommand(context.Background(), "echo hello", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "hello" {
		t.Errorf("got %q, want %q", output, "hello")
	}
}

func TestExecuteShell_Failure(t *testing.T) {
	_, err := executeCommand(context.Background(), "false", "", 10)
	if err == nil {
		t.Error("expected error for failing command")
	}
}

func TestExecuteShell_Timeout(t *testing.T) {
	ctx := context.Background()
	start := time.Now()
	_, err := executeCommand(ctx, "sleep 10", "", 1)
	elapsed := time.Since(start)
	if err == nil {
		t.Error("expected timeout error")
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestExecuteHTTP_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content-type")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	output, err := executeCommand(context.Background(), srv.URL, `{"key":"val"}`, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "ok") {
		t.Errorf("expected ok in output, got %q", output)
	}
}

func TestExecuteHTTP_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := executeCommand(context.Background(), srv.URL, "", 10)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestExecuteHTTP_ConnectionRefused(t *testing.T) {
	_, err := executeCommand(context.Background(), "http://127.0.0.1:1", "", 5)
	if err == nil {
		t.Error("expected connection refused error")
	}
}

func TestExecuteShell_MultiWord(t *testing.T) {
	output, err := executeCommand(context.Background(), "echo hello world", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "hello world" {
		t.Errorf("got %q, want %q", output, "hello world")
	}
}

func TestExecuteShell_EmptyCommand(t *testing.T) {
	_, err := executeCommand(context.Background(), "", "", 10)
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func BenchmarkExecuteShell(b *testing.B) {
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		_, _ = executeCommand(ctx, "echo bench", "", 10)
	}
}
