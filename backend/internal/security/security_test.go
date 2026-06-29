package security

import (
	"net/http/httptest"
	"testing"
)

func TestManagerAllowsConfiguredRoles(t *testing.T) {
	manager := NewManager("admin-token:admin,viewer-token:viewer")
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("authorization", "Bearer viewer-token")
	if !manager.Allowed(req, PermRead) {
		t.Fatal("viewer should read")
	}
	if manager.Allowed(req, PermWrite) {
		t.Fatal("viewer should not write")
	}
	req.Header.Set("x-api-key", "admin-token")
	if !manager.Allowed(req, PermAdmin) {
		t.Fatal("admin should admin")
	}
}

func TestDisabledManagerAllowsDevelopmentAccess(t *testing.T) {
	manager := NewManager("")
	req := httptest.NewRequest("POST", "/", nil)
	if !manager.Allowed(req, PermAdmin) {
		t.Fatal("disabled auth should allow development access")
	}
}
