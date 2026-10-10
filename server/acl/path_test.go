package acl

import (
	"reflect"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "/"},
		{"/", "/"},
		{"/a/../b", "/b"},
		{"/a//b/", "/a/b"},
		{"docs", "/docs"},
		{"./x", "/x"},
	}
	for _, tc := range tests {
		if got := NormalizePath(tc.in); got != tc.want {
			t.Fatalf("NormalizePath(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestAncestorPaths(t *testing.T) {
	got := AncestorPaths("/a/b/c")
	want := []string{"/a/b/c", "/a/b", "/a", "/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	rootFirst := AncestorPathsRootFirst("/a/b")
	wantRoot := []string{"/", "/a", "/a/b"}
	if !reflect.DeepEqual(rootFirst, wantRoot) {
		t.Fatalf("root-first %#v want %#v", rootFirst, wantRoot)
	}
}

func TestIsSelfOrUnder(t *testing.T) {
	tests := []struct {
		p, root string
		want    bool
	}{
		{"/home/docs", "/home/docs", true},
		{"/home/docs/a", "/home/docs", true},
		{"/home", "/home/docs", false},
		{"/home/docs2", "/home/docs", false},
		{"/anything", "/", true},
		{"/", "/", true},
	}
	for _, tc := range tests {
		if got := IsSelfOrUnder(tc.p, tc.root); got != tc.want {
			t.Fatalf("IsSelfOrUnder(%q,%q)=%v want %v", tc.p, tc.root, got, tc.want)
		}
	}
}

func TestLikeUnder(t *testing.T) {
	if got := LikeUnder("/"); got != "/%" {
		t.Fatalf("root: %q", got)
	}
	if got := LikeUnder("/docs"); got != "/docs/%" {
		t.Fatalf("docs: %q", got)
	}
}
