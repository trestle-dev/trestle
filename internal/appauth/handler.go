package appauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/httperr"
	"github.com/trestle-dev/trestle/internal/store"
	"golang.org/x/crypto/argon2"
)

type Handler struct {
	db    store.Executor
	admin *adminauth.Handler
	now   func() time.Time
}
type credentials struct{ Email, Password string }
type refreshInput struct {
	RefreshToken string `json:"refreshToken"`
}

func New(db any, admin *adminauth.Handler) *Handler {
	return &Handler{db: store.Adapt(db), admin: admin, now: time.Now}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/register":
		h.register(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/login":
		h.login(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/refresh":
		h.refresh(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/logout":
		h.logout(w, r)
	case strings.HasPrefix(r.URL.Path, "/admin/v1/app-users"):
		h.adminRoutes(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	email, ok := email(in.Email)
	if !ok {
		writeError(w, 422, "validation_failed", "The request could not be applied.")
		return
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		writeError(w, 422, "validation_failed", err.Error())
		return
	}
	id := "usr_" + token(18)
	now := h.now().UTC().Format(time.RFC3339Nano)
	if _, err = h.db.ExecContext(r.Context(), "INSERT INTO _trestle_app_users(id,email,password_hash,created_at) VALUES(?,?,?,?)", id, email, hash, now); err != nil {
		writeError(w, 409, "registration_unavailable", "The account could not be created.")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "email": email, "verificationRequired": true})
}
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	normalized, _ := email(in.Email)
	var id, stored, hash string
	var disabled sql.NullString
	err := h.db.QueryRowContext(r.Context(), "SELECT id,email,password_hash,disabled_at FROM _trestle_app_users WHERE email=?", normalized).Scan(&id, &stored, &hash, &disabled)
	if err != nil || disabled.Valid || !verifyPassword(hash, in.Password) {
		dummyVerify(in.Password)
		writeError(w, 401, "invalid_credentials", "The email or password is incorrect.")
		return
	}
	h.issue(w, r, id, stored)
}
func (h *Handler) issue(w http.ResponseWriter, r *http.Request, userID, email string) {
	raw := token(32)
	sum := sha256.Sum256([]byte(raw))
	id := "aps_" + token(18)
	now := h.now().UTC()
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), "INSERT INTO _trestle_app_sessions(id,user_id,refresh_hash,created_at,expires_at) VALUES(?,?,?,?,?)", id, userID, sum[:], now.Format(time.RFC3339Nano), now.Add(30*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	access, err := h.createAccessTx(r, tx, id, userID)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	writeJSON(w, 200, map[string]any{"userId": userID, "email": email, "accessToken": access, "accessExpiresIn": 900, "refreshToken": raw, "expiresIn": 2592000})
}
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var in refreshInput
	if !decode(w, r, &in) {
		return
	}
	sum := sha256.Sum256([]byte(in.RefreshToken))
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	defer tx.Rollback()
	var sessionID, userID, email, expires string
	var revoked, disabled sql.NullString
	err = tx.QueryRowContext(r.Context(), `SELECT s.id,u.id,u.email,s.expires_at,s.revoked_at,u.disabled_at FROM _trestle_app_sessions s JOIN _trestle_app_users u ON u.id=s.user_id WHERE s.refresh_hash=?`, sum[:]).Scan(&sessionID, &userID, &email, &expires, &revoked, &disabled)
	expiry, _ := time.Parse(time.RFC3339Nano, expires)
	if err != nil || revoked.Valid || disabled.Valid || !expiry.After(h.now()) {
		writeError(w, 401, "invalid_refresh", "The refresh token is invalid.")
		return
	}
	next := token(32)
	nextSum := sha256.Sum256([]byte(next))
	nextID := "aps_" + token(18)
	now := h.now().UTC()
	result, err := tx.ExecContext(r.Context(), "UPDATE _trestle_app_sessions SET revoked_at=?,replaced_by=? WHERE id=? AND revoked_at IS NULL", now.Format(time.RFC3339Nano), nextID, sessionID)
	affected, _ := result.RowsAffected()
	if err != nil || affected != 1 {
		writeError(w, 401, "invalid_refresh", "The refresh token is invalid.")
		return
	}
	_, err = tx.ExecContext(r.Context(), "INSERT INTO _trestle_app_sessions(id,user_id,refresh_hash,created_at,expires_at) VALUES(?,?,?,?,?)", nextID, userID, nextSum[:], now.Format(time.RFC3339Nano), now.Add(30*24*time.Hour).Format(time.RFC3339Nano))
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	access, err := h.createAccessTx(r, tx, nextID, userID)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	writeJSON(w, 200, map[string]any{"userId": userID, "email": email, "accessToken": access, "accessExpiresIn": 900, "refreshToken": next, "expiresIn": 2592000})
}

// createAccessTx inserts the short-lived access-token row on the caller's
// transaction so a session and its access token commit together. A failed
// access insert rolls back the whole login or refresh: no orphaned session,
// and no rotated refresh token without a usable access token.
func (h *Handler) createAccessTx(r *http.Request, tx store.Transaction, sessionID, userID string) (string, error) {
	raw := "ta_" + token(24)
	sum := sha256.Sum256([]byte(raw))
	_, err := tx.ExecContext(r.Context(), "INSERT INTO _trestle_app_access(token_hash,session_id,user_id,expires_at) VALUES(?,?,?,?)", sum[:], sessionID, userID, h.now().UTC().Add(15*time.Minute).Format(time.RFC3339Nano))
	return raw, err
}
func (h *Handler) Authenticate(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ta_") {
		return "", false
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))))
	var userID, expires string
	var revoked, disabled sql.NullString
	err := h.db.QueryRowContext(r.Context(), `SELECT a.user_id,a.expires_at,s.revoked_at,u.disabled_at FROM _trestle_app_access a JOIN _trestle_app_sessions s ON s.id=a.session_id JOIN _trestle_app_users u ON u.id=a.user_id WHERE a.token_hash=?`, sum[:]).Scan(&userID, &expires, &revoked, &disabled)
	expiry, _ := time.Parse(time.RFC3339Nano, expires)
	return userID, err == nil && !revoked.Valid && !disabled.Valid && expiry.After(h.now())
}
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var in refreshInput
	if !decode(w, r, &in) {
		return
	}
	sum := sha256.Sum256([]byte(in.RefreshToken))
	result, err := h.db.ExecContext(r.Context(), "UPDATE _trestle_app_sessions SET revoked_at=? WHERE refresh_hash=? AND revoked_at IS NULL", h.now().UTC().Format(time.RFC3339Nano), sum[:])
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	// An unknown or already-revoked token is an idempotent success; a durable
	// write failure above is the only case that must not report success.
	_ = result
	w.WriteHeader(204)
}
func (h *Handler) adminRoutes(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.admin.Authorize(r, r.Method != http.MethodGet); !ok {
		writeError(w, 403, "authorization_denied", "The request is not authorized.")
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/admin/v1/app-users" {
		rows, err := h.db.QueryContext(r.Context(), "SELECT id,email,verified_at,disabled_at,created_at FROM _trestle_app_users ORDER BY created_at DESC LIMIT 200")
		if err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id, email, created string
			var verified, disabled sql.NullString
			rows.Scan(&id, &email, &verified, &disabled, &created)
			items = append(items, map[string]any{"id": id, "email": email, "verified": verified.Valid, "disabled": disabled.Valid, "createdAt": created})
		}
		writeJSON(w, 200, map[string]any{"items": items})
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 5 && r.Method == http.MethodPost && parts[4] == "disable" {
		now := h.now().UTC().Format(time.RFC3339Nano)
		tx, err := h.db.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(r.Context(), "UPDATE _trestle_app_users SET disabled_at=? WHERE id=?", now, parts[3]); err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
		if _, err := tx.ExecContext(r.Context(), "UPDATE _trestle_app_sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL", now, parts[3]); err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
		if err := tx.Commit(); err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
		w.WriteHeader(204)
		return
	}
	http.NotFound(w, r)
}
func email(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	return value, err == nil && parsed.Address == value && len(value) <= 254
}
func token(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func hashPassword(password string) (string, error) {
	if len([]rune(password)) < 7 {
		return "", fmt.Errorf("password must be at least 7 characters")
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false
	}
	salt, e1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, e2 := base64.RawStdEncoding.DecodeString(parts[5])
	if e1 != nil || e2 != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
func dummyVerify(password string) { argon2.IDKey([]byte(password), make([]byte, 16), 1, 8*1024, 1, 32) }
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		writeError(w, 400, "invalid_json", "The request body is invalid.")
		return false
	}
	return true
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, httperr.New(code, message, w.Header().Get("X-Request-ID")))
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
