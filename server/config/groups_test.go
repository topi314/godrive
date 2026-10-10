package config

import "testing"

func TestMapOIDCGroups(t *testing.T) {
	g := AuthGroups{
		Admin:  "admin",
		Access: "godrive",
		Map: map[string]string{
			"ldap-linux": "linux",
			"family":     "family",
		},
	}
	got := g.MapOIDCGroups([]string{"vpn", "admin", "ldap-linux", "godrive", "linux", "admin"})
	want := []string{"admin", "linux", "godrive"}
	if len(got) != len(want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v want %#v", got, want)
		}
	}
	avail := g.AvailableGroups()
	if !contains(avail, "admin") || !contains(avail, "godrive") || !contains(avail, "linux") || !contains(avail, "family") {
		t.Fatalf("available %#v", avail)
	}
	if g.IsAvailableGroup("vpn") {
		t.Fatal("vpn should not be available")
	}
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
