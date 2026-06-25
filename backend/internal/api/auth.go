package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"goGateway/internal/auth"
	"goGateway/internal/models"
)

// AuthHandler implements login, token refresh, logout, and user CRUD.
type AuthHandler struct {
	cfg      auth.Config
	sessions *auth.SessionStore
	deps     Deps
}

func NewAuthHandler(d Deps, cfg auth.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg, sessions: auth.NewSessionStore(d.DB), deps: d}
}

// Mount wires auth + user routes.  Public endpoints (login/refresh) are
// unauthenticated.  User management requires superadmin.
func (h *AuthHandler) Mount(r chi.Router) {
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)

	// Authenticated routes: require a valid access token.
	r.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware(h.cfg, h.sessions))
		r.Post("/logout", h.logout)

		// User management — superadmin only.
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("superadmin"))
			r.Get("/users", h.listUsers)
			r.Post("/users", h.createUser)
			r.Put("/users/{id}", h.updateUser)
			r.Delete("/users/{id}", h.deleteUser)
		})
	})
}

// login accepts {username, password} and returns access+refresh tokens.
func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}

	var u models.User
	if err := h.deps.DB.GetContext(r.Context(), &u,
		`SELECT id, username, password_hash, role, enabled FROM users WHERE username = ?`,
		req.Username,
	); err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !u.Enabled {
		writeErr(w, http.StatusForbidden, "account disabled")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	sessionID, rawRT, err := h.sessions.Create(
		r.Context(), u.ID,
		r.Header.Get("User-Agent"), remoteAddr(r),
		h.cfg.RefreshDuration,
	)
	if err != nil {
		log.Printf("auth: create session: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	accessToken, err := h.cfg.IssueAccessToken(u.ID, u.Username, u.Role, sessionID)
	if err != nil {
		log.Printf("auth: issue token: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": rawRT,
		"session_id":    sessionID,
		"token_type":    "Bearer",
		"expires_in":    int(h.cfg.AccessDuration.Seconds()),
		"role":          u.Role,
	})
}

// refresh accepts {session_id, refresh_token} and issues a new access token
// without requiring the user's password again.
func (h *AuthHandler) refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID    string `json:"session_id"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}

	row, err := h.sessions.Validate(r.Context(), req.SessionID, req.RefreshToken)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid or expired session")
		return
	}

	var u models.User
	if err := h.deps.DB.GetContext(r.Context(), &u,
		`SELECT id, username, role, enabled FROM users WHERE id = ?`, row.UserID,
	); err != nil || !u.Enabled {
		writeErr(w, http.StatusUnauthorized, "account not found or disabled")
		return
	}

	accessToken, err := h.cfg.IssueAccessToken(u.ID, u.Username, u.Role, req.SessionID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   int(h.cfg.AccessDuration.Seconds()),
		"role":         u.Role,
	})
}

// logout revokes the session whose ID is embedded in the access token's jti.
func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r.Context())
	if claims != nil {
		if err := h.sessions.Revoke(r.Context(), claims.ID); err != nil {
			log.Printf("auth: revoke session %s: %v", claims.ID, err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- User management (superadmin only) ---

func (h *AuthHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	var users []models.User
	if err := h.deps.DB.SelectContext(r.Context(), &users,
		`SELECT id, username, email, full_name, role, enabled, created_at, updated_at
		   FROM users ORDER BY id`,
	); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *AuthHandler) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if req.Role == "" {
		req.Role = models.RoleViewer
	}
	if !models.ValidRole(req.Role) {
		writeErr(w, http.StatusBadRequest, "invalid role")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	res, err := h.deps.DB.ExecContext(r.Context(),
		`INSERT INTO users (username, password_hash, email, full_name, role)
		 VALUES (?, ?, ?, ?, ?)`,
		req.Username, hash, req.Email, req.FullName, req.Role,
	)
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *AuthHandler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
		Enabled  *bool  `json:"enabled"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if req.Role != "" && !models.ValidRole(req.Role) {
		writeErr(w, http.StatusBadRequest, "invalid role")
		return
	}
	if req.Password != "" {
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		if _, err := h.deps.DB.ExecContext(r.Context(),
			`UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			hash, id,
		); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	if _, err := h.deps.DB.ExecContext(r.Context(),
		`UPDATE users SET email=?, full_name=?, role=?, enabled=?,
		 updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		req.Email, req.FullName, req.Role, enabled, id,
	); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Force-revoke all sessions when the account is disabled.
	if req.Enabled != nil && !*req.Enabled {
		if err := h.sessions.RevokeAllForUser(r.Context(), id); err != nil {
			log.Printf("auth: revoke sessions for user %d: %v", id, err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.deps.DB.ExecContext(r.Context(),
		`DELETE FROM users WHERE id = ?`, id,
	); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func remoteAddr(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}
