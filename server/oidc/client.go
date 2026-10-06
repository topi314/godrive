package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const LoginFlowTTL = 10 * time.Minute

type LoginFlow struct {
	Nonce        string
	CodeVerifier string
	Created      time.Time
}

type Discovery struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserinfoEndpoint      string   `json:"userinfo_endpoint"`
	JwksURI               string   `json:"jwks_uri"`
	IntrospectionEndpoint string   `json:"introspection_endpoint"`
	RevocationEndpoint    string   `json:"revocation_endpoint"`
	EndSessionEndpoint    string   `json:"end_session_endpoint"`
	PAREndpoint           string   `json:"pushed_authorization_request_endpoint"`
	RequirePAR            bool     `json:"require_pushed_authorization_requests"`
	IssParameterSupported bool     `json:"authorization_response_iss_parameter_supported"`
	CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
}

type Client struct {
	Verifier  *gooidc.IDTokenVerifier
	Config    *oauth2.Config
	Provider  *gooidc.Provider
	Discovery Discovery
	States    map[string]LoginFlow
	StatesMu  sync.Mutex
}

type parResponse struct {
	RequestURI string `json:"request_uri"`
	ExpiresIn  int    `json:"expires_in"`
}

type introspectResponse struct {
	Active bool   `json:"active"`
	Sub    string `json:"sub"`
}

func (c *Client) LoadDiscovery(endSessionOverride string) {
	if c == nil || c.Provider == nil {
		return
	}
	var disc Discovery
	if err := c.Provider.Claims(&disc); err != nil {
		slog.Warn("oidc discovery claims", slog.Any("err", err))
		return
	}
	if disc.EndSessionEndpoint == "" {
		disc.EndSessionEndpoint = strings.TrimSpace(endSessionOverride)
	}
	c.Discovery = disc
	slog.Info("oidc discovery",
		slog.String("issuer", disc.Issuer),
		slog.String("authorization", disc.AuthorizationEndpoint),
		slog.String("token", disc.TokenEndpoint),
		slog.String("userinfo", disc.UserinfoEndpoint),
		slog.String("jwks", disc.JwksURI),
		slog.String("introspection", disc.IntrospectionEndpoint),
		slog.String("revocation", disc.RevocationEndpoint),
		slog.String("end_session", disc.EndSessionEndpoint),
		slog.String("par", disc.PAREndpoint),
		slog.Bool("require_par", disc.RequirePAR),
		slog.Bool("iss_parameter", disc.IssParameterSupported),
	)
}

func (c *Client) PutLoginFlow(state string, flow LoginFlow) {
	c.StatesMu.Lock()
	defer c.StatesMu.Unlock()
	now := time.Now()
	for k, v := range c.States {
		if now.Sub(v.Created) > LoginFlowTTL {
			delete(c.States, k)
		}
	}
	c.States[state] = flow
}

func (c *Client) TakeLoginFlow(state string) (LoginFlow, bool) {
	c.StatesMu.Lock()
	defer c.StatesMu.Unlock()
	flow, ok := c.States[state]
	if ok {
		delete(c.States, state)
	}
	if !ok || time.Since(flow.Created) > LoginFlowTTL {
		return LoginFlow{}, false
	}
	return flow, true
}

func Nonce(nonce string) oauth2.AuthCodeOption {
	return oauth2.SetAuthURLParam("nonce", nonce)
}

func (c *Client) AuthorizationURL(ctx context.Context, state, nonce, verifier string) (string, error) {
	if c.Discovery.PAREndpoint != "" {
		uri, err := c.pushedAuthorizationRequest(ctx, state, nonce, verifier)
		if err == nil && uri != "" {
			u, err := url.Parse(c.Config.Endpoint.AuthURL)
			if err != nil {
				return "", err
			}
			q := u.Query()
			q.Set("client_id", c.Config.ClientID)
			q.Set("request_uri", uri)
			u.RawQuery = q.Encode()
			return u.String(), nil
		}
		if c.Discovery.RequirePAR {
			return "", err
		}
		slog.Warn("par failed, falling back to authorization query", slog.Any("err", err))
	}
	return c.Config.AuthCodeURL(state,
		Nonce(nonce),
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
	), nil
}

func (c *Client) pushedAuthorizationRequest(ctx context.Context, state, nonce, verifier string) (string, error) {
	form := url.Values{}
	form.Set("response_type", "code")
	form.Set("client_id", c.Config.ClientID)
	form.Set("redirect_uri", c.Config.RedirectURL)
	form.Set("scope", strings.Join(c.Config.Scopes, " "))
	form.Set("state", state)
	form.Set("nonce", nonce)
	form.Set("code_challenge", oauth2.S256ChallengeFromVerifier(verifier))
	form.Set("code_challenge_method", "S256")
	form.Set("access_type", "offline")
	resp, err := c.formPost(ctx, c.Discovery.PAREndpoint, form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("par: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out parResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("par decode: %w", err)
	}
	if out.RequestURI == "" {
		return "", fmt.Errorf("par: empty request_uri")
	}
	return out.RequestURI, nil
}

func (c *Client) formPost(ctx context.Context, endpoint string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.Config.ClientID, c.Config.ClientSecret)
	client := &http.Client{Timeout: 15 * time.Second}
	return client.Do(req)
}

func (c *Client) Revoke(ctx context.Context, token, hint string) {
	if c == nil || c.Discovery.RevocationEndpoint == "" || token == "" {
		return
	}
	form := url.Values{}
	form.Set("token", token)
	if hint != "" {
		form.Set("token_type_hint", hint)
	}
	resp, err := c.formPost(ctx, c.Discovery.RevocationEndpoint, form)
	if err != nil {
		slog.Warn("oidc revoke failed", slog.String("hint", hint), slog.Any("err", err))
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		slog.Warn("oidc revoke status", slog.String("hint", hint), slog.Int("status", resp.StatusCode))
	}
}

func (c *Client) Introspect(ctx context.Context, token, hint string) (bool, error) {
	if c == nil || c.Discovery.IntrospectionEndpoint == "" || token == "" {
		return false, nil
	}
	form := url.Values{}
	form.Set("token", token)
	if hint != "" {
		form.Set("token_type_hint", hint)
	}
	resp, err := c.formPost(ctx, c.Discovery.IntrospectionEndpoint, form)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("introspect: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out introspectResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return false, err
	}
	return out.Active, nil
}

func (c *Client) LogoutURL(idToken, postLogoutRedirect string) string {
	if c == nil || c.Discovery.EndSessionEndpoint == "" {
		return ""
	}
	u, err := url.Parse(c.Discovery.EndSessionEndpoint)
	if err != nil {
		return ""
	}
	q := u.Query()
	if idToken != "" {
		q.Set("id_token_hint", idToken)
	}
	q.Set("client_id", c.Config.ClientID)
	if postLogoutRedirect != "" {
		q.Set("post_logout_redirect_uri", postLogoutRedirect)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func AuthorizationIssValid(got, issuer string, required bool) bool {
	got = strings.TrimSpace(got)
	if got == "" {
		return !required
	}
	return got == issuer
}
