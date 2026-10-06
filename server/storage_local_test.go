package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStorageResolveNoEscape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "store")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// Sibling that must never be reachable via prefix confusion.
	sibling := filepath.Join(parent, "storeevil")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := &localStorage{root: root}
	if _, err := st.resolve("/ok.txt"); err != nil {
		t.Fatalf("in-root resolve: %v", err)
	}

	// Simulate what a buggy HasPrefix check would allow: abs path under storeevil.
	// resolve() only accepts logical paths; ensure .. cannot walk to sibling.
	got, err := st.resolve("/../storeevil/secret.txt")
	if err == nil {
		// Normalized to /storeevil/secret.txt under root — file shouldn't exist there.
		if _, statErr := os.Stat(got); statErr == nil {
			t.Fatalf("resolved outside storage to %s", got)
		}
	}

	rel, err := filepath.Rel(root, sibling)
	if err != nil {
		t.Fatal(err)
	}
	if rel == ".." || filepath.HasPrefix(rel, "..") {
		// expected: sibling is outside
	}
	if under, _ := filepath.Rel(root, filepath.Join(sibling, "secret.txt")); under == ".." || len(under) >= 2 && under[:2] == ".." {
		// good — outside
	}
}
