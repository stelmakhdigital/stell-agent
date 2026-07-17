package tools

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBashDeniedWithoutTrust(t *testing.T) {
	env := &Env{Workspace: t.TempDir(), BashDeny: true}
	_, err := (bashTool{}).Call(context.Background(), env, map[string]any{"command": "echo hi"})
	if err == nil {
		t.Fatal("expected deny error")
	}
}

func TestBashAllowedWithApprove(t *testing.T) {
	env := &Env{Workspace: t.TempDir(), BashAutoApprove: true}
	res, err := (bashTool{}).Call(context.Background(), env, map[string]any{"command": "echo hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Content == "" {
		t.Fatal("empty output")
	}
}

func TestRunBashDeniedWithoutTrust(t *testing.T) {
	rt := NewRuntime(Env{Workspace: t.TempDir(), BashDeny: true})
	_, err := rt.RunBash(context.Background(), "echo hi")
	if err == nil {
		t.Fatal("expected deny error")
	}
}

func TestBashRejectsFilesystemScan(t *testing.T) {
	env := &Env{Workspace: t.TempDir(), BashAutoApprove: true}
	for _, cmd := range []string{
		`find / -name "bash_test.go"`,
		`find /* -name x`,
		`cat missing 2>/dev/null || echo "not found"; find / -name "bash_test.go" 2>/dev/null | head -5`,
		`grep -r pattern /`,
	} {
		_, err := (bashTool{}).Call(context.Background(), env, map[string]any{"command": cmd})
		if err == nil {
			t.Fatalf("expected filesystem scan rejection for %q", cmd)
		}
	}
}

func TestBashAllowsWorkspaceFind(t *testing.T) {
	env := &Env{Workspace: t.TempDir(), BashAutoApprove: true}
	for _, cmd := range []string{
		`find . -name "*.go"`,
		`mkdir -p cmd && find ./cmd -type f`,
		`ls /tmp`,
	} {
		if _, err := (bashTool{}).Call(context.Background(), env, map[string]any{"command": cmd}); err != nil {
			t.Fatalf("command %q should be allowed: %v", cmd, err)
		}
	}
}

func TestBashNonZeroExitIsToolError(t *testing.T) {
	env := &Env{Workspace: t.TempDir(), BashAutoApprove: true}
	_, err := (bashTool{}).Call(context.Background(), env, map[string]any{"command": "ls /nonexistent-dir-xyz"})
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if !strings.Contains(err.Error(), "command failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBashHeartbeatForSilentCommand(t *testing.T) {
	old := bashHeartbeatInterval
	bashHeartbeatInterval = 50 * time.Millisecond
	defer func() { bashHeartbeatInterval = old }()

	var mu sync.Mutex
	var updates []string
	ctx := WithProgress(context.Background(), func(partial string) {
		mu.Lock()
		updates = append(updates, partial)
		mu.Unlock()
	})
	_, err := runBashCommand(ctx, t.TempDir(), "sleep 0.3", ProgressFrom(ctx))
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	var sawMarker bool
	for _, u := range updates {
		if strings.Contains(u, "[running ") {
			sawMarker = true
		}
	}
	if !sawMarker {
		t.Fatalf("expected heartbeat marker in progress updates, got %v", updates)
	}
}
