// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAndRole(t *testing.T) {
	dir := t.TempDir()
	a, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.Role("") != RoleAdmin {
		t.Fatal("open access should be admin")
	}
	if err := a.Update("short", "admin-token-ok"); err == nil {
		t.Fatal("expected short token to fail")
	}
	if err := a.Update("user-token-ok", ""); err == nil {
		t.Fatal("expected user without admin to fail")
	}
	if err := a.Update("same-token", "same-token"); err == nil {
		t.Fatal("expected equal tokens to fail")
	}
	if err := a.Update("user-token-ok", "admin-token-ok"); err != nil {
		t.Fatal(err)
	}
	if a.Role("nope") != RoleNone {
		t.Fatal("bad token")
	}
	if a.Role("user-token-ok") != RoleUser {
		t.Fatal("user")
	}
	if a.Role("admin-token-ok") != RoleAdmin {
		t.Fatal("admin")
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Role("admin-token-ok") != RoleAdmin {
		t.Fatal("reload")
	}
}

func TestOpenAccessDoesNotStripPath(t *testing.T) {
	a := &Access{}
	req := httptest.NewRequest(http.MethodGet, "/not-a-token/api/balances", nil)
	token, path := a.Resolve(req)
	if token != "" || path != "/not-a-token/api/balances" {
		t.Fatalf("open access rewrote path: %q %q", token, path)
	}
}

func TestResolvePathPrefix(t *testing.T) {
	a := &Access{}
	if err := a.SetMemory("user-token-ok", "admin-token-ok"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/admin-token-ok/api/balances", nil)
	token, path := a.Resolve(req)
	if token != "admin-token-ok" || path != "/api/balances" {
		t.Fatalf("got %q %q", token, path)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/send", nil)
	req.Header.Set("Authorization", "Bearer user-token-ok")
	token, path = a.Resolve(req)
	if token != "user-token-ok" || path != "/api/send" {
		t.Fatalf("bearer got %q %q", token, path)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/health?token=admin-token-ok", nil)
	token, path = a.Resolve(req)
	if token != "admin-token-ok" || path != "/api/health" {
		t.Fatalf("query got %q %q", token, path)
	}
}
