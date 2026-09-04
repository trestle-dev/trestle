package appauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/trestle-cv/trestle/internal/adminauth"
	"github.com/trestle-cv/trestle/internal/appreg"
	"github.com/trestle-cv/trestle/internal/audit"
	"github.com/trestle-cv/trestle/internal/httperr"
	"github.com/trestle-cv/trestle/internal/requestmeta"
	"github.com/trestle-cv/trestle/internal/store"
	"golang.org/x/crypto/argon2"
)

type Handler struct {
	db       store.Executor
	admin    *adminauth.Handler
	reg      *appreg.Service
	now      func() time.Time
	limiter  *limiter
	invLim   *limiter
	reqLim   *limiter
	provider store.Provider
}
type credentials struct{ Email, Password string }
type refreshInput struct {
	RefreshToken string `json:"refreshToken"`
}

func New(db any, admin *adminauth.Handler) *Handler {
	exec := store.Adapt(db)
	return &Handler{db: exec, admin: admin, reg: appreg.New(exec, nil), provider: exec.Dialect().Provider(), now: time.Now, limiter: newLimiter(10, time.Minute), invLim: newLimiter(10, time.Minute), reqLim: newLimiter(5, time.Minute)}
}

func (h *Handler) SetAudit(audit *audit.Handler) { h.reg = appreg.New(h.db, audit) }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/capability":
		h.capability(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/register":
		h.register(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/invite/accept":
		h.inviteAccept(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/access-request":
		h.accessRequest(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/login":
		h.login(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/refresh":
		h.refresh(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/logout":
		h.logout(w, r)
	case strings.HasPrefix(r.URL.Path, "/admin/v1/app-users") || strings.HasPrefix(r.URL.Path, "/admin/v1/app-registration"):
		h.adminRoutes(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (h *Handler) capability(w http.ResponseWriter, r *http.Request) {
	policy, err := h.reg.CurrentPolicy(r.Context())
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	writeJSON(w, 200, map[string]any{"registration": map[string]any{"flow": appreg.FlowForPolicy(policy.Name), "emailDelivery": "manual"}})
}
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	key := clientKey(r)
	if !h.limiter.Allow(key, h.now()) {
		writeError(w, 429, "rate_limited", "Too many attempts. Try again later.")
		return
	}
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	email, ok := email(in.Email)
	if !ok {
		writeError(w, 422, "validation_failed", "The request could not be applied.")
		return
	}
	// Validate and hash the password BEFORE acquiring any lock.
	hash, err := hashPassword(in.Password)
	if err != nil {
		writeError(w, 422, "validation_failed", err.Error())
		return
	}
	// Serialize registration against concurrent policy changes on the
	// singleton policy row. The policy is read under the lock so a concurrent
	// policy change that wins the lock makes a later registration fail.
	var newID string
	err = store.WithSerializedLock(r.Context(), h.db, func(tx store.Transaction) error {
		policy, perr := h.policyForTx(r.Context(), tx)
		if perr != nil {
			return perr
		}
		if policy != appreg.PolicyOpen {
			return fmt.Errorf("policy_%s", policy)
		}
		idRaw, err := secureToken(18)
		if err != nil {
			return err
		}
		createdID := "usr_" + idRaw
		now := h.now().UTC().Format(time.RFC3339Nano)
		if _, ierr := tx.ExecContext(r.Context(), "INSERT INTO _trestle_app_users(id,email,password_hash,created_at) VALUES(?,?,?,?)", createdID, email, hash, now); ierr != nil {
			return errors.New("insert_user")
		}
		if err := h.reg.AuditUserCreation(r.Context(), tx, createdID, email, "register"); err != nil {
			return err
		}
		newID = createdID
		return nil
	})
	if err != nil {
		msg := err.Error()
		switch {
		case strings.HasPrefix(msg, "policy_"):
			writeError(w, 403, msg, "The request could not be applied.")
		case strings.Contains(msg, "entropy_source_unavailable") || strings.Contains(msg, "audit_not_configured"):
			writeError(w, 503, "registration_temporarily_unavailable", "The request could not be completed.")
		case errors.Is(err, store.ErrLockExhausted):
			writeError(w, 503, "registration_temporarily_unavailable", "The request could not be completed.")
		default:
			writeError(w, 409, "registration_unavailable", "The account could not be created.")
		}
		return
	}
	writeJSON(w, 201, map[string]any{"id": newID, "email": email, "verificationRequired": true})
}

// policyForTx reads the singleton policy row on the caller's transaction after
// acquiring the serialization lock. On PostgreSQL the read locks the row with
// SELECT ... FOR UPDATE; on SQLite the BEGIN IMMEDIATE write lock already
// serializes, so a plain read observes the locked state.
func (h *Handler) policyForTx(ctx context.Context, tx store.Transaction) (string, error) {
	if h.provider == store.Postgres {
		if _, err := tx.ExecContext(ctx, "SELECT id FROM _trestle_app_registration_policy WHERE id=1 FOR UPDATE"); err != nil {
			return "", err
		}
	}
	var policy string
	if err := tx.QueryRowContext(ctx, "SELECT policy FROM _trestle_app_registration_policy WHERE id=1").Scan(&policy); err != nil {
		return "", err
	}
	return policy, nil
}

type inviteAcceptInput struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// inviteAccept consumes an email-bound invitation atomically and creates the
// application user with the password. The password is validated and hashed
// before the serialized transaction (for self_register) so no long-lived lock
// is held during Argon2id. self_register requires the current policy to be
// invite; activate is honored under every policy. Single-use token serialization
// is mandatory.
func (h *Handler) inviteAccept(w http.ResponseWriter, r *http.Request) {
	if !h.invLim.Allow(clientKey(r), h.now()) {
		writeError(w, 429, "rate_limited", "Too many attempts. Try again later.")
		return
	}
	var in inviteAcceptInput
	if !decode(w, r, &in) {
		return
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		writeError(w, 422, "validation_failed", err.Error())
		return
	}
	tokenHash := sha256.Sum256([]byte(in.Token))
	var acceptedEmail, acceptedID string
	err = store.WithSerializedLock(r.Context(), h.db, func(tx store.Transaction) error {
		// Atomic single-use claim: obtain invitation data only when it is
		// unused, not revoked and not expired. On PostgreSQL the serialized
		// transaction plus this read contend on the token row; on SQLite the
		// BEGIN IMMEDIATE write lock serializes consumers.
		var id, kind, boundEmail string
		var expires string
		var used, revoked, existingUser sql.NullString
		row := tx.QueryRowContext(r.Context(), `SELECT id,kind,email,expires_at,used_at,revoked_at,user_id FROM _trestle_app_invitations WHERE token_hash=?`, tokenHash[:])
		if err := row.Scan(&id, &kind, &boundEmail, &expires, &used, &revoked, &existingUser); err != nil {
			return errors.New("invalid_invitation")
		}
		expiry, perr := time.Parse(time.RFC3339Nano, expires)
		if perr != nil || used.Valid || revoked.Valid || !expiry.After(h.now()) || existingUser.Valid {
			return errors.New("invalid_invitation")
		}
		if kind == "self_register" {
			// Serialize against policy changes on the singleton policy row.
			policy, err := h.policyForTx(r.Context(), tx)
			if err != nil {
				return err
			}
			if policy != appreg.PolicyInvite {
				return errors.New("invalid_invitation")
			}
		}
		idRaw, err := secureToken(18)
		if err != nil {
			return err
		}
		newID := "usr_" + idRaw
		now := h.now().UTC().Format(time.RFC3339Nano)
		if _, ierr := tx.ExecContext(r.Context(), "INSERT INTO _trestle_app_users(id,email,password_hash,created_at) VALUES(?,?,?,?)", newID, boundEmail, hash, now); ierr != nil {
			return errors.New("invalid_invitation") // email conflict
		}
		if _, ierr := tx.ExecContext(r.Context(), "UPDATE _trestle_app_invitations SET used_at=?,user_id=? WHERE id=? AND used_at IS NULL AND revoked_at IS NULL", now, newID, id); ierr != nil {
			return ierr
		}
		if _, ierr := tx.ExecContext(r.Context(), "UPDATE _trestle_app_invitations SET revoked_at=? WHERE email=? AND kind=? AND used_at IS NULL AND revoked_at IS NULL AND id<>?", now, boundEmail, "activate", id); ierr != nil {
			return ierr
		}
		if err := h.reg.AuditUserCreation(r.Context(), tx, newID, boundEmail, kind); err != nil {
			return err
		}
		acceptedEmail = boundEmail
		acceptedID = newID
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrLockExhausted) || strings.Contains(err.Error(), "entropy_source_unavailable") || strings.Contains(err.Error(), "audit_not_configured") {
			writeError(w, 503, "registration_temporarily_unavailable", "The request could not be completed.")
			return
		}
		writeError(w, 400, "invalid_invitation", "The invitation is invalid.")
		return
	}
	writeJSON(w, 201, map[string]any{"id": acceptedID, "email": acceptedEmail})
}

type accessRequestInput struct {
	Email string `json:"email"`
}

// accessRequest records a bounded pending access request under the approval
// policy. It always presents the same generic outcome to the caller.
func (h *Handler) accessRequest(w http.ResponseWriter, r *http.Request) {
	if !h.reqLim.Allow(clientKey(r), h.now()) {
		writeError(w, 429, "rate_limited", "Too many attempts. Try again later.")
		return
	}
	var in accessRequestInput
	if !decode(w, r, &in) {
		return
	}
	policy, err := h.reg.CurrentPolicy(r.Context())
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	if policy.Name != appreg.PolicyApproval {
		writeError(w, 403, "registration_approval_required", "Registration by approval is not available.")
		return
	}
	if err := h.reg.SubmitAccessRequest(r.Context(), in.Email); err != nil {
		if err.Error() == "policy_not_approval" {
			writeError(w, 403, "registration_approval_required", "Registration by approval is not available.")
			return
		}
		if err.Error() == "invalid_email" {
			writeError(w, 422, "validation_failed", "The request could not be applied.")
			return
		}
		// Genuine storage failure: keep the public response indistinguishable
		// for enumeration safety, but surface a bounded server-side metric so
		// operators can observe it. No submitted email is logged.
		slog.Warn("access request storage failure", "count", h.reg.StorageFailures())
	}
	writeJSON(w, 202, map[string]any{"status": "requested"})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeError(w, 403, "origin_denied", "The request origin is not allowed.")
		return
	}
	key := clientKey(r)
	if !h.limiter.Allow(key, h.now()) {
		writeError(w, 429, "rate_limited", "Too many attempts. Try again later.")
		return
	}
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
	raw, err := secureToken(32)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	sum := sha256.Sum256([]byte(raw))
	idRaw, err := secureToken(18)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	id := "aps_" + idRaw
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
	next, err := secureToken(32)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	nextSum := sha256.Sum256([]byte(next))
	nextIDRaw, err := secureToken(18)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	nextID := "aps_" + nextIDRaw
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
	raw, err := secureToken(24)
	if err != nil {
		return "", err
	}
	token := "ta_" + raw
	sum := sha256.Sum256([]byte(token))
	_, err = tx.ExecContext(r.Context(), "INSERT INTO _trestle_app_access(token_hash,session_id,user_id,expires_at) VALUES(?,?,?,?)", sum[:], sessionID, userID, h.now().UTC().Add(15*time.Minute).Format(time.RFC3339Nano))
	return token, err
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
	if r.URL.Path == "/admin/v1/app-registration" && r.Method == http.MethodGet {
		policy, err := h.reg.CurrentPolicy(r.Context())
		if err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
		invitations, _ := h.reg.ListInvitations(r.Context())
		requests, _ := h.reg.ListAccessRequests(r.Context())
		pending := 0
		for _, req := range requests {
			if req.Status == "pending" {
				pending++
			}
		}
		writeJSON(w, 200, map[string]any{"policy": policy.Name, "setAt": policy.SetAt, "invitationsOpen": countOpen(invitations), "requestsPending": pending})
		return
	}
	if r.URL.Path == "/admin/v1/app-registration/policy" && r.Method == http.MethodPut {
		principal, _ := h.admin.Authorize(r, true)
		var in struct {
			Policy      string `json:"policy"`
			ConfirmOpen bool   `json:"confirmOpen"`
		}
		if !decode(w, r, &in) {
			return
		}
		policy, err := h.reg.SetPolicy(r.Context(), principal.AdminID, in.Policy, in.ConfirmOpen)
		if err != nil {
			writeError(w, 400, err.Error(), "The request could not be applied.")
			return
		}
		writeJSON(w, 200, map[string]any{"policy": policy.Name, "setAt": policy.SetAt})
		return
	}
	if r.URL.Path == "/admin/v1/app-registration/activation-base-url" && r.Method == http.MethodGet {
		value, err := h.reg.ActivationBaseURL(r.Context())
		if err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
		writeJSON(w, 200, map[string]any{"activationBaseUrl": value})
		return
	}
	if r.URL.Path == "/admin/v1/app-registration/activation-base-url" && r.Method == http.MethodPut {
		principal, _ := h.admin.Authorize(r, true)
		var in struct {
			ActivationBaseURL string `json:"activationBaseUrl"`
		}
		if !decode(w, r, &in) {
			return
		}
		normalized, err := h.reg.SetActivationBaseURL(r.Context(), principal.AdminID, in.ActivationBaseURL)
		if err != nil {
			writeError(w, 400, err.Error(), "The request could not be applied.")
			return
		}
		// Return the canonical serialized value that was persisted ("" when
		// cleared) so the client adopts exactly what the link builder uses.
		writeJSON(w, 200, map[string]any{"activationBaseUrl": normalized})
		return
	}
	if r.URL.Path == "/admin/v1/app-registration/invitations" && r.Method == http.MethodPost {
		principal, _ := h.admin.Authorize(r, true)
		var in struct {
			Kind    string `json:"kind"`
			Email   string `json:"email"`
			Request string `json:"requestId"`
		}
		if !decode(w, r, &in) {
			return
		}
		inv, err := h.reg.CreateInvitation(r.Context(), principal.AdminID, in.Kind, in.Email, in.Request)
		if err != nil {
			code := err.Error()
			status := 400
			if code == "user_already_exists" {
				status = 409
			}
			writeError(w, status, code, "The request could not be applied.")
			return
		}
		writeJSON(w, 201, inv)
		return
	}
	if r.URL.Path == "/admin/v1/app-registration/invitations" && r.Method == http.MethodGet {
		invitations, err := h.reg.ListInvitations(r.Context())
		if err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
		writeJSON(w, 200, map[string]any{"items": invitations})
		return
	}
	if r.URL.Path == "/admin/v1/app-registration/requests" && r.Method == http.MethodGet {
		requests, err := h.reg.ListAccessRequests(r.Context())
		if err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
		writeJSON(w, 200, map[string]any{"items": requests})
		return
	}
	if len(parts) == 6 && parts[2] == "app-registration" {
		switch {
		case parts[3] == "invitations" && parts[5] == "revoke" && r.Method == http.MethodPost:
			principal, _ := h.admin.Authorize(r, true)
			if err := h.reg.RevokeInvitation(r.Context(), principal.AdminID, parts[4]); err != nil {
				writeError(w, 500, "internal_error", "The request could not be completed.")
				return
			}
			w.WriteHeader(204)
			return
		case parts[3] == "requests" && parts[5] == "approve" && r.Method == http.MethodPost:
			principal, _ := h.admin.Authorize(r, true)
			inv, err := h.reg.ApproveRequest(r.Context(), principal.AdminID, parts[4])
			if err != nil {
				writeError(w, 409, "already_decided", "The request has already been decided.")
				return
			}
			writeJSON(w, 201, inv)
			return
		case parts[3] == "requests" && parts[5] == "reject" && r.Method == http.MethodPost:
			principal, _ := h.admin.Authorize(r, true)
			if err := h.reg.RejectRequest(r.Context(), principal.AdminID, parts[4]); err != nil {
				writeError(w, 409, "already_decided", "The request has already been decided.")
				return
			}
			w.WriteHeader(204)
			return
		case parts[3] == "requests" && parts[5] == "reissue" && r.Method == http.MethodPost:
			principal, _ := h.admin.Authorize(r, true)
			// The email is derived from the approved request row; the caller
			// cannot substitute a different email.
			inv, err := h.reg.ReissueInvitation(r.Context(), principal.AdminID, parts[4])
			if err != nil {
				writeError(w, 400, err.Error(), "The request could not be applied.")
				return
			}
			writeJSON(w, 201, inv)
			return
		}
	}
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

// entropyReader is the checked entropy source for the password salt and every
// secureToken value. It is a package-level var so tests can inject a failing or
// short-reading source; both hashPassword and secureToken require the buffer to
// be filled exactly and propagate the failure before any durable mutation.
var entropyReader io.Reader = rand.Reader

// secureToken returns a random URL-safe token, requiring the entropy source to
// fill the requested buffer exactly. It is the checked randomness helper for
// every security-sensitive value (session and access token secrets, user and
// session identifiers). Entropy-source failure propagates before any durable
// mutation.
func secureToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(entropyReader, b); err != nil {
		return "", errors.New("entropy_source_unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashPassword(password string) (string, error) {
	if len([]rune(password)) < 7 {
		return "", fmt.Errorf("password must be at least 7 characters")
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(entropyReader, salt); err != nil {
		return "", errors.New("entropy_source_unavailable")
	}
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

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return origin == requestmeta.Scheme(r)+"://"+r.Host
}
func clientKey(r *http.Request) string {
	return requestmeta.ClientIP(r)
}

// limiter is a small fixed-window per-key attempt limiter.
type limiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	attempts map[string][]time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, attempts: map[string][]time.Time{}}
}
func (l *limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-l.window)
	items := l.attempts[key][:0]
	for _, at := range l.attempts[key] {
		if at.After(cut) {
			items = append(items, at)
		}
	}
	if len(items) >= l.max {
		l.attempts[key] = items
		return false
	}
	l.attempts[key] = append(items, now)
	return true
}
func (l *limiter) Clear(key string) { l.mu.Lock(); delete(l.attempts, key); l.mu.Unlock() }

func countOpen(invitations []appreg.Invitation) int {
	n := 0
	for _, it := range invitations {
		if it.UsedAt == nil && it.RevokedAt == nil {
			n++
		}
	}
	return n
}

// SetRequestLimit adjusts the access-request rate limiter (used by tests and
// by deployments that need a different abuse threshold).
func (h *Handler) SetRequestLimit(n int) { h.reqLim = newLimiter(n, time.Minute) }
