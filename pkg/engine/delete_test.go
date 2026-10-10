package engine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Deleting one session removes its tree/log/events while every other
// session stays intact; the LATEST pointer is cleared only when it named
// the deleted session.
func TestStorage_DeleteSession(t *testing.T) {
	// LATEST points at s2, the session we delete.
	s := newPruneFixture(t, "s2")
	dir := s.baseDir

	art, err := s.DeleteSession("s2")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if art.ID != "s2" {
		t.Fatalf("artifact id=%q", art.ID)
	}
	// tree + log + events + the LATEST pointer.
	if len(art.Paths) != 4 {
		t.Fatalf("removed paths=%v, want tree/log/events/LATEST", art.Paths)
	}
	if art.Bytes <= 0 {
		t.Fatal("bytes freed should be reported")
	}
	for _, name := range []string{"s2.json", filepath.Join("logs", "s2.log"), filepath.Join("events", "s2.jsonl"), "LATEST"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone (err=%v)", name, err)
		}
	}
	if _, err := s.LoadTree("s2"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("s2 should not load, got %v", err)
	}
	// Second delete reports not-exist.
	if _, err := s.DeleteSession("s2"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repeat delete: %v, want os.ErrNotExist", err)
	}
	// Sibling sessions survive untouched.
	for _, id := range []string{"s1", "s3"} {
		if !s.TreeExists(id) {
			t.Fatalf("%s should survive", id)
		}
		if _, err := os.Stat(filepath.Join(dir, "events", id+".jsonl")); err != nil {
			t.Fatalf("%s events should survive: %v", id, err)
		}
	}
	if infos, err := s.ListSessions(); err != nil || len(infos) != 2 {
		t.Fatalf("list after delete: %v %v", infos, err)
	}
}

// Deleting a session that LATEST does not point at leaves the pointer in
// place, so a bare -resume still reaches the intended session.
func TestStorage_DeleteSessionKeepsLatestForOthers(t *testing.T) {
	s := newPruneFixture(t, "s3")
	if _, err := s.DeleteSession("s1"); err != nil {
		t.Fatal(err)
	}
	if got := s.latestID(); got != "s3" {
		t.Fatalf("LATEST=%q, want s3", got)
	}
}

// Traversal-shaped ids must be rejected before any path operation runs.
func TestStorage_DeleteSessionRejectsBadIDs(t *testing.T) {
	s := newPruneFixture(t, "s3")
	for _, bad := range []string{"", ".", "..", "../s3", "s3/../s2", "a/b", `a\b`, "s3\n", "s 3", string(make([]byte, 129))} {
		if _, err := s.DeleteSession(bad); err == nil {
			t.Fatalf("bad id %q must be rejected", bad)
		}
	}
	// Rejection must not have removed anything.
	for _, id := range []string{"s1", "s2", "s3"} {
		if !s.TreeExists(id) {
			t.Fatalf("%s must survive a rejected delete", id)
		}
	}
}

func TestValidSessionID(t *testing.T) {
	good := []string{"sess_20261010_085558", "abc", "A-Z_0-9", "x-y_z"}
	for _, id := range good {
		if !ValidSessionID(id) {
			t.Fatalf("%q should be valid", id)
		}
	}
	bad := []string{"", ".", "..", "a..b", "a/b", "../x", "a.b", "a b", "a:b", "a?b"}
	for _, id := range bad {
		if ValidSessionID(id) {
			t.Fatalf("%q should be invalid", id)
		}
	}
}
