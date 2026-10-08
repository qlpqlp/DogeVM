// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package config

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const fileName = "connect_config.json"

// Role is the access level of a request.
type Role int

const (
	RoleNone Role = iota
	RoleUser
	RoleAdmin
)

// Access stores the optional user and admin API tokens.
// When both are empty, every request is treated as admin (local open access).
type Access struct {
	mu    sync.RWMutex
	path  string
	user  string
	admin string
}

type file struct {
	UserToken  string `json:"user_token"`
	AdminToken string `json:"admin_token"`
}

// Load reads connect_config.json from dataDir. Missing file means open access.
// DOGEVM_USER_TOKEN and DOGEVM_ADMIN_TOKEN override the file for this process.
func Load(dataDir string) (*Access, error) {
	a := &Access{path: filepath.Join(dataDir, fileName)}
	raw, err := os.ReadFile(a.path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && len(strings.TrimSpace(string(raw))) > 0 {
		var f file
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, err
		}
		a.user = strings.TrimSpace(f.UserToken)
		a.admin = strings.TrimSpace(f.AdminToken)
	}
	if v := strings.TrimSpace(os.Getenv("DOGEVM_USER_TOKEN")); v != "" {
		a.user = v
	}
	if v := strings.TrimSpace(os.Getenv("DOGEVM_ADMIN_TOKEN")); v != "" {
		a.admin = v
	}
	if err := validatePair(a.user, a.admin); err != nil {
		return nil, err
	}
	return a, nil
}

// SetMemory replaces the in-memory tokens without writing the file.
func (a *Access) SetMemory(user, admin string) error {
	user = strings.TrimSpace(user)
	admin = strings.TrimSpace(admin)
	if err := validatePair(user, admin); err != nil {
		return err
	}
	a.mu.Lock()
	a.user = user
	a.admin = admin
	a.mu.Unlock()
	return nil
}

// Update saves tokens to disk and applies them immediately.
func (a *Access) Update(user, admin string) error {
	user = strings.TrimSpace(user)
	admin = strings.TrimSpace(admin)
	if err := validatePair(user, admin); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(file{UserToken: user, AdminToken: admin}, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(a.path), 0o700); err != nil {
		return err
	}
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, a.path); err != nil {
		return err
	}
	a.mu.Lock()
	a.user = user
	a.admin = admin
	a.mu.Unlock()
	return nil
}

// Tokens returns the current user and admin tokens.
func (a *Access) Tokens() (user, admin string) {
	if a == nil {
		return "", ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.user, a.admin
}

// Required reports whether a token must be presented.
func (a *Access) Required() bool {
	if a == nil {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.user != "" || a.admin != ""
}

// Role classifies a presented token.
func (a *Access) Role(token string) Role {
	if a == nil {
		return RoleAdmin
	}
	a.mu.RLock()
	user, admin := a.user, a.admin
	a.mu.RUnlock()
	if user == "" && admin == "" {
		return RoleAdmin
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return RoleNone
	}
	if tokenEqual(token, admin) {
		return RoleAdmin
	}
	if tokenEqual(token, user) {
		return RoleUser
	}
	return RoleNone
}

// Resolve reads a token from Authorization: Bearer, X-Token, ?token=,
// or a /{token}/api/... path prefix. path is the API path to dispatch.
func (a *Access) Resolve(r *http.Request) (token, path string) {
	path = r.URL.Path
	if h := r.Header.Get("Authorization"); len(h) >= 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:]), path
	}
	if h := strings.TrimSpace(r.Header.Get("X-Token")); h != "" {
		return h, path
	}
	if q := strings.TrimSpace(r.URL.Query().Get("token")); q != "" {
		return q, path
	}
	trimmed := strings.Trim(path, "/")
	seg, rest, ok := strings.Cut(trimmed, "/")
	if !ok || seg == "" {
		return "", path
	}
	if !strings.HasPrefix(rest, "api/") && rest != "api" {
		return "", path
	}
	if !a.knownToken(seg) {
		return "", path
	}
	return seg, "/" + rest
}

func (a *Access) knownToken(token string) bool {
	if a == nil || token == "" {
		return false
	}
	a.mu.RLock()
	user, admin := a.user, a.admin
	a.mu.RUnlock()
	return tokenEqual(token, user) || tokenEqual(token, admin)
}

func validatePair(user, admin string) error {
	if user == "" && admin == "" {
		return nil
	}
	if admin == "" {
		return errors.New("set an admin token too, or leave both empty")
	}
	if len(admin) < 8 || (user != "" && len(user) < 8) {
		return errors.New("tokens must be at least 8 characters")
	}
	if strings.ContainsAny(user, " \t\r\n") || strings.ContainsAny(admin, " \t\r\n") {
		return errors.New("tokens cannot contain spaces")
	}
	if user != "" && user == admin {
		return errors.New("user token and admin token must be different")
	}
	return nil
}

func tokenEqual(a, b string) bool {
	if a == "" || b == "" || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
