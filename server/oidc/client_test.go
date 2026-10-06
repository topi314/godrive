package oidc

import (
	"net/url"
	"testing"

	"golang.org/x/oauth2"
)

func TestAuthorizationIssValid(t *testing.T) {
	issuer := "https://auth.example.com"
	if !AuthorizationIssValid("https://auth.example.com", issuer, true) {
		t.Fatal("matching iss")
	}
	if AuthorizationIssValid("https://evil.example", issuer, true) {
		t.Fatal("mismatched iss")
	}
	if AuthorizationIssValid("", issuer, true) {
		t.Fatal("missing iss when required")
	}
	if !AuthorizationIssValid("", issuer, false) {
		t.Fatal("missing iss when optional")
	}
}

func TestLogoutURL(t *testing.T) {
	c := &Client{
		Config: &oauth2.Config{ClientID: "godrive"},
		Discovery: Discovery{
			EndSessionEndpoint: "https://auth.example.com/oidc/end-session",
		},
	}
	got := c.LogoutURL("id.jwt", "http://localhost:3000/")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("id_token_hint") != "id.jwt" || q.Get("client_id") != "godrive" {
		t.Fatalf("query %v", q)
	}
	if q.Get("post_logout_redirect_uri") != "http://localhost:3000/" {
		t.Fatalf("post_logout %q", q.Get("post_logout_redirect_uri"))
	}
}
