package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloneCopiesHistoryAndCheckpoints(t *testing.T) {
	m := NewManager()
	src := m.GetOrCreate("src")
	src.Messages = []map[string]any{
		{"role": "user", "content": "hello"},
	}
	src.Checkpoints["src-u1"] = []map[string]any{
		{"role": "user", "content": "hello"},
	}
	src.UserTurns = 1

	if ok := m.Clone("src", "dst"); !ok {
		t.Fatalf("expected clone to succeed")
	}

	dst, ok := m.Get("dst")
	if !ok {
		t.Fatalf("expected dst session")
	}
	if dst.UserTurns != 1 {
		t.Fatalf("expected user turns=1, got %d", dst.UserTurns)
	}
	if len(dst.Messages) != 1 || dst.Messages[0]["content"] != "hello" {
		t.Fatalf("unexpected dst messages: %#v", dst.Messages)
	}
	if len(dst.Checkpoints) != 1 {
		t.Fatalf("expected checkpoint copy")
	}
}

func TestNewForkIDDeterministic(t *testing.T) {
	m := NewManager()
	if got := m.NewForkID("abc"); got != "abc#fork-1" {
		t.Fatalf("unexpected first fork id: %s", got)
	}
	if got := m.NewForkID("abc"); got != "abc#fork-2" {
		t.Fatalf("unexpected second fork id: %s", got)
	}
}

func TestEnablePersistenceLoadsExistingSessions(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "sessions.json")

	m1 := NewManager()
	if err := m1.EnablePersistence(store); err != nil {
		t.Fatalf("enable persistence: %v", err)
	}
	s := m1.GetOrCreate("persisted")
	s.Messages = []map[string]any{{"role": "user", "content": "hi"}}
	m1.Snapshot("persisted", "persisted-u1")

	m2 := NewManager()
	if err := m2.EnablePersistence(store); err != nil {
		t.Fatalf("enable persistence: %v", err)
	}
	loaded, ok := m2.Get("persisted")
	if !ok {
		t.Fatalf("expected persisted session")
	}
	if len(loaded.Checkpoints) == 0 {
		t.Fatalf("expected checkpoints to load")
	}
}

func TestFileSnapshotAndRewindRestoresFilesystem(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(file, []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	m := NewManager()
	s := m.GetOrCreate("default")
	s.Messages = []map[string]any{{"role": "user", "content": "hello"}}
	m.Snapshot("default", "u1")
	if err := m.SnapshotFiles("default", "u1", dir); err != nil {
		t.Fatalf("snapshot files: %v", err)
	}

	if err := os.WriteFile(file, []byte("v2"), 0o644); err != nil {
		t.Fatalf("write file v2: %v", err)
	}
	extra := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(extra, []byte("extra"), 0o644); err != nil {
		t.Fatalf("write extra file: %v", err)
	}

	if err := m.RewindFiles("default", "u1", dir); err != nil {
		t.Fatalf("rewind files: %v", err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read rewound file: %v", err)
	}
	if string(got) != "v1" {
		t.Fatalf("expected v1, got %q", string(got))
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Fatalf("expected extra file removed, stat err=%v", err)
	}
}
