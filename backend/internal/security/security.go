package security

import (
	"net/http"
	"strings"
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

type Permission string

const (
	PermRead  Permission = "read"
	PermWrite Permission = "write"
	PermAdmin Permission = "admin"
)

type Manager struct {
	keys map[string]Role
}

func NewManager(spec string) Manager {
	keys := map[string]Role{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		token, roleText, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		token = strings.TrimSpace(token)
		role := Role(strings.TrimSpace(strings.ToLower(roleText)))
		if token != "" && validRole(role) {
			keys[token] = role
		}
	}
	return Manager{keys: keys}
}

func (m Manager) Enabled() bool {
	return len(m.keys) > 0
}

func (m Manager) RoleFromRequest(r *http.Request) (Role, bool) {
	if !m.Enabled() {
		return RoleAdmin, true
	}
	token := strings.TrimSpace(r.Header.Get("x-api-key"))
	if token == "" {
		auth := strings.TrimSpace(r.Header.Get("authorization"))
		token = strings.TrimPrefix(strings.TrimPrefix(auth, "Bearer "), "bearer ")
	}
	role, ok := m.keys[token]
	return role, ok
}

func (m Manager) Allowed(r *http.Request, permission Permission) bool {
	role, ok := m.RoleFromRequest(r)
	if !ok {
		return false
	}
	return RoleAllows(role, permission)
}

func RoleAllows(role Role, permission Permission) bool {
	switch permission {
	case PermRead:
		return role == RoleViewer || role == RoleEditor || role == RoleAdmin
	case PermWrite:
		return role == RoleEditor || role == RoleAdmin
	case PermAdmin:
		return role == RoleAdmin
	default:
		return false
	}
}

func validRole(role Role) bool {
	return role == RoleViewer || role == RoleEditor || role == RoleAdmin
}
