package templates

import (
	"time"

	"github.com/topi314/godrive/godrive/auth"
)

type UserType string

const (
	UserTypeAdmin UserType = "admin"
	UserTypeUser  UserType = "user"
	UserTypeGuest UserType = "guest"
	UserTypeNone  UserType = ""
)

type User struct {
	ID     string
	Name   string
	Groups []string
	Email  string
	Home   string
	Type   UserType
}

type File struct {
	IsDir           bool
	Path            string
	Dir             string
	Name            string
	Size            int64
	Description     string
	Date            time.Time
	Owners          []string
	OwnerIDs        []string
	Permissions     auth.Permissions
	FilePermissions []Permissions
}

type ToggleState int

const (
	ToggleStateUnset ToggleState = iota
	ToggleStateOn
	ToggleStateOff
)

type Permissions struct {
	Path       string
	ObjectType auth.ObjectType
	Object     string
	ObjectName string
	Allow      auth.Permissions
	Deny       auth.Permissions
	Map        map[string]ToggleState
}

var UnsetPermissions = map[string]ToggleState{
	"read":               ToggleStateUnset,
	"create":             ToggleStateUnset,
	"update":             ToggleStateUnset,
	"delete":             ToggleStateUnset,
	"update_permissions": ToggleStateUnset,
	"share":              ToggleStateUnset,
}
