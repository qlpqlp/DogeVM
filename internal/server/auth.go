// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/DogecoinVM/dogevm-connect/internal/config"
)

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		token := ""
		if s.Access != nil {
			token, path = s.Access.Resolve(r)
		}
		if path != r.URL.Path {
			cloned := r.Clone(r.Context())
			cloned.URL.Path = path
			r = cloned
		}
		if !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if path == "/api/health" || path == "/api/auth" {
			next.ServeHTTP(w, r)
			return
		}
		role := config.RoleAdmin
		if s.Access != nil {
			role = s.Access.Role(token)
		}
		if role == config.RoleNone {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error":        "token required",
				"authRequired": true,
			})
			return
		}
		if requiresAdmin(path) && role != config.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error": "admin token required",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requiresAdmin(path string) bool {
	switch path {
	case "/api/config", "/api/wallet/export", "/api/wallet/remove":
		return true
	default:
		return false
	}
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	required := s.Access != nil && s.Access.Required()
	token := ""
	if s.Access != nil {
		token, _ = s.Access.Resolve(r)
	}
	role := "open"
	if s.Access != nil {
		switch s.Access.Role(token) {
		case config.RoleAdmin:
			if required {
				role = "admin"
			}
		case config.RoleUser:
			role = "user"
		default:
			if required {
				role = "none"
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authRequired": required,
		"role":         role,
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if s.Access == nil {
		writeErr(w, http.StatusInternalServerError, errors.New("config store unavailable"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		user, admin := s.Access.Tokens()
		writeJSON(w, http.StatusOK, map[string]any{
			"user_token":   user,
			"admin_token":  admin,
			"authRequired": user != "" || admin != "",
		})
	case http.MethodPost:
		var body struct {
			UserToken  string `json:"user_token"`
			AdminToken string `json:"admin_token"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, errors.New("invalid JSON"))
			return
		}
		if err := s.Access.Update(body.UserToken, body.AdminToken); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		user, admin := s.Access.Tokens()
		writeJSON(w, http.StatusOK, map[string]any{
			"saved":        true,
			"user_token":   user,
			"admin_token":  admin,
			"authRequired": user != "" || admin != "",
		})
	default:
		writeErr(w, http.StatusMethodNotAllowed, errors.New("GET or POST only"))
	}
}
