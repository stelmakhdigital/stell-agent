package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathRejectsOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	_, _, err := ResolvePath(ws, "../outside.txt")
	if err == nil {
		t.Fatal("expected error for path outside workspace")
	}
}

func TestResolvePathSymlinkEscape(t *testing.T) {
	ws := t.TempDir()
	secret := t.TempDir()
	secretFile := filepath.Join(secret, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(ws, "link.txt")
	if err := os.Symlink(secretFile, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	_, _, err := ResolvePath(ws, "link.txt")
	if err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func TestResolveOutputPathDefault(t *testing.T) {
	ws := t.TempDir()
	abs, rel, err := ResolveOutputPath(ws, "", "export.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "export.jsonl" {
		t.Fatalf("rel=%q", rel)
	}
	if filepath.Base(abs) != "export.jsonl" {
		t.Fatalf("abs=%q", abs)
	}
}
