package eventlog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendBatchCapsAndPersists(t *testing.T) {
	dir := t.TempDir()
	l := New[int](dir, "ev.jsonl", 3)
	l.Append()           // empty batch: no-op, no file
	l.Append(1, 2, 3, 4) // over cap: keeps the newest 3
	l.Append(5)

	if got := l.Recent(0); len(got) != 3 || got[0] != 5 || got[2] != 3 {
		t.Fatalf("Recent = %v, want [5 4 3]", got)
	}
	if got := New[int](dir, "ev.jsonl", 3).Recent(0); len(got) != 3 || got[0] != 5 {
		t.Fatalf("reloaded = %v, want [5 4 3]", got)
	}
	info, err := os.Stat(filepath.Join(dir, "ev.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}
