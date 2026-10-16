package server

import "testing"

func TestZipSelectionPaths(t *testing.T) {
	got := zipSelectionPaths("/home/alice", []string{"a.txt", "../x", "docs", "a.txt", "", "sub/x"})
	want := []string{"/home/alice/a.txt", "/home/alice/docs"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if zipSelectionPaths("/docs", nil) != nil {
		t.Fatal("expected nil for empty names")
	}
}

func TestZipPathSelected(t *testing.T) {
	sel := []string{"/home/alice/a.txt", "/home/alice/docs"}
	if !zipPathSelected("/home/alice/a.txt", sel) {
		t.Fatal("file should match")
	}
	if !zipPathSelected("/home/alice/docs/nested/x", sel) {
		t.Fatal("child should match")
	}
	if zipPathSelected("/home/alice/b.txt", sel) {
		t.Fatal("other file should not match")
	}
	if !zipPathSelected("/anything", nil) {
		t.Fatal("empty selection includes all")
	}
}

func TestZipEntryNameSelected(t *testing.T) {
	sel := []string{"/home/alice/a.txt", "/home/alice/docs"}
	name, ok := zipEntryNameSelected(sel, "/home/alice/a.txt", false)
	if !ok || name != "a.txt" {
		t.Fatalf("got %q %v", name, ok)
	}
	name, ok = zipEntryNameSelected(sel, "/home/alice/docs/nested/x", false)
	if !ok || name != "docs/nested/x" {
		t.Fatalf("got %q %v", name, ok)
	}
	name, ok = zipEntryNameSelected(sel, "/home/alice/docs", true)
	if !ok || name != "docs/" {
		t.Fatalf("got %q %v", name, ok)
	}
	if _, ok := zipEntryNameSelected(sel, "/home/alice/other", false); ok {
		t.Fatal("expected miss")
	}
}
