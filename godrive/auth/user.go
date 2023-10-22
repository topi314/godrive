package auth

import (
	"context"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Action string

const (
	ActionDeny  Action = "deny"
	ActionAllow Action = "allow"
	ActionLogin Action = "login"
)

type authKey struct{}

var userInfoKey = authKey{}

type UserInfo struct {
	oidc.UserInfo
	Home     string   `json:"home"`
	Audience []string `json:"aud"`
	Groups   []string `json:"groups"`
	Username string   `json:"preferred_username"`
}

func (u *UserInfo) IsGuest() bool {
	return u.Subject == "guest"
}

func GetUserInfo(r *http.Request) *UserInfo {
	userInfo := r.Context().Value(userInfoKey)
	if userInfo == nil {
		return &UserInfo{
			UserInfo: oidc.UserInfo{
				Subject: "guest",
				Email:   "guest@localhost",
			},
			Audience: []string{"godrive"},
			Groups:   []string{"guest"},
			Username: "guest",
		}
	}
	return userInfo.(*UserInfo)
}

func SetUserInfo(r *http.Request, userInfo *UserInfo) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userInfoKey, userInfo))
}
