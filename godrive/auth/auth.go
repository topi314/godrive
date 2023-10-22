package auth

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/topi314/godrive/godrive/database"
	"golang.org/x/exp/maps"
	"golang.org/x/oauth2"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

type Config struct {
	Secure               bool          `cfg:"secure"`
	Issuer               string        `cfg:"issuer"`
	ClientID             string        `cfg:"client_id"`
	ClientSecret         string        `cfg:"client_secret"`
	RedirectURL          string        `cfg:"redirect_url"`
	LogoutURL            string        `cfg:"logout_url"`
	RefreshTokenLifespan time.Duration `cfg:"refresh_token_lifespan"`
	DefaultHome          string        `cfg:"default_home"`
	Groups               Groups        `cfg:"groups"`
}

func (c Config) String() string {
	return fmt.Sprintf("\n  Secure: %t\n  Issuer: %s\n  ClientID: %s\n  ClientSecret: %s\n  RedirectURL: %s\n  LogoutURL: %s\n  RefreshTokenLifespan: %s\n  DefaultHome: %s\n  Groups: %s",
		c.Secure,
		c.Issuer,
		c.ClientID,
		strings.Repeat("*", len(c.ClientSecret)),
		c.RedirectURL,
		c.LogoutURL,
		c.RefreshTokenLifespan,
		c.DefaultHome,
		c.Groups,
	)
}

type Groups struct {
	Admin       string `cfg:"admin"`
	User        string `cfg:"user"`
	Guest       string `cfg:"guest"`
	AllowGuests bool   `cfg:"allow_guests"`
}

func (c Groups) String() string {
	return fmt.Sprintf("\n    Admin: %s\n    User: %s\n    Guest: %s\n    AllowGuests: %t",
		c.Admin,
		c.User,
		c.Guest,
		c.AllowGuests,
	)
}

func New(cfg *Config, db *database.DB) (*Auth, error) {
	var (
		provider *oidc.Provider
		verifier *oidc.IDTokenVerifier
		config   *oauth2.Config
		err      error
	)
	if cfg != nil {
		provider, err = oidc.NewProvider(context.Background(), cfg.Issuer)
		if err != nil {
			return nil, err
		}

		verifier = provider.Verifier(&oidc.Config{
			ClientID: cfg.ClientID,
		})

		config = &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "groups", "email", "profile", oidc.ScopeOfflineAccess},
		}
	}
	return &Auth{
		cfg:      cfg,
		provider: provider,
		verifier: verifier,
		config:   config,
		states:   make(map[string]loginState),
		db:       db,
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

type Auth struct {
	cfg      *Config
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	config   *oauth2.Config

	states   map[string]loginState
	statesMu sync.Mutex

	db   *database.DB
	rand *rand.Rand
}

func (a *Auth) Config() *oauth2.Config {
	return a.config
}

func (a *Auth) Verifier() *oidc.IDTokenVerifier {
	return a.verifier
}

func (a *Auth) GetPermissions(ctx context.Context, info *UserInfo, file database.File) (Permissions, error) {
	if info.Subject == file.UserID {
		return PermissionsAll, nil
	}

	return a.GetFilePermissions(ctx, file.Path, info)
}

func (a *Auth) GetMultiplePermissions(ctx context.Context, info *UserInfo, files []database.File) (map[string]Permissions, error) {
	if a.cfg == nil || a.IsAdmin(info.Groups) {
		perms := make(map[string]Permissions, len(files))
		for _, file := range files {
			perms[file.Path] = PermissionsAll
		}
		return perms, nil
	}

	finalPerms := make(map[string]Permissions, len(files))
	unknownPaths := make([]string, 0)
	for _, file := range files {
		if info.Subject == file.UserID {
			finalPerms[file.Path] = PermissionsAll
			continue
		}

		unknownPaths = append(unknownPaths, file.Path)
	}

	perms, err := a.GetFilesPermissions(ctx, unknownPaths, info)
	if err != nil {
		return nil, err
	}

	maps.Copy(finalPerms, perms)
	return finalPerms, nil
}

func (a *Auth) HasAccess(info *UserInfo) bool {
	if a.cfg == nil {
		return true
	}

	if !a.cfg.Groups.AllowGuests && a.IsGuest(info.Groups) {
		return false
	}

	return a.IsAdmin(info.Groups) || a.IsUser(info.Groups) || a.IsGuest(info.Groups)
}

func (a *Auth) IsAdmin(groups []string) bool {
	if a.cfg == nil {
		return true
	}
	return slices.Contains(groups, a.cfg.Groups.Admin)
}

func (a *Auth) IsUser(groups []string) bool {
	if a.cfg == nil {
		return false
	}
	return slices.Contains(groups, a.cfg.Groups.User)
}

func (a *Auth) IsGuest(groups []string) bool {
	if a.cfg == nil {
		return false
	}
	return slices.Contains(groups, a.cfg.Groups.Guest)
}
