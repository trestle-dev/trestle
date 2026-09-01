package appreg

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/trestle-dev/trestle/internal/audit"
	"github.com/trestle-dev/trestle/internal/store"
)

// Fixed initial lifetimes for application registration (first implementation).
const (
	InvitationLifetime        = 7 * 24 * time.Hour  // 7 days
	AccessRequestPendingTTL   = 14 * 24 * time.Hour // 14 days
	AccessRequestRetentionTTL = 30 * 24 * time.Hour // 30 days
	CleanupBatch              = 200
)

// Service implements the application registration policy, invitation and
// access-request model. It is a pure store layer used by the appauth and admin
// HTTP handlers and by the first-run setup flow.
type Service struct {
	store      store.Executor
	audit      *audit.Handler
	now        func() time.Time
	randReader func([]byte) (int, error)
	failures   atomic.Uint64
}

func New(store store.Executor, audit *audit.Handler) *Service {
	return &Service{store: store, audit: audit, now: time.Now, randReader: rand.Read}
}

// StorageFailures returns the number of genuine access-request storage
// failures observed (bounded server-side metric; the public response stays
// enumeration-safe).
func (s *Service) StorageFailures() uint64 { return s.failures.Load() }

// Policy constants.
const (
	PolicyOpen     = "open"
	PolicyInvite   = "invite"
	PolicyApproval = "approval"
	PolicyClosed   = "closed"
)

func ValidPolicy(p string) bool {
	switch p {
	case PolicyOpen, PolicyInvite, PolicyApproval, PolicyClosed:
		return true
	}
	return false
}

// FlowForPolicy returns the public client flow a policy presents.
func FlowForPolicy(policy string) string {
	switch policy {
	case PolicyOpen:
		return "register"
	case PolicyInvite:
		return "invite"
	case PolicyApproval:
		return "request"
	default:
		return "closed"
	}
}

// Policy is the durable deployment-wide registration policy.
type Policy struct {
	Name  string `json:"policy"`
	SetAt string `json:"setAt"`
}

// CurrentPolicy reads the singleton policy row (read-only path used by
// capability checks and ordinary policy evaluation).
func (s *Service) CurrentPolicy(ctx context.Context) (Policy, error) {
	var name, setAt string
	if err := s.store.QueryRowContext(ctx, "SELECT policy,set_at FROM _trestle_app_registration_policy WHERE id=1").Scan(&name, &setAt); err != nil {
		return Policy{}, err
	}
	return Policy{Name: name, SetAt: setAt}, nil
}

// SetPolicy changes the policy, serializing on the singleton row and writing
// the audit fact in the same transaction. Open requires explicit confirmation.
func (s *Service) SetPolicy(ctx context.Context, actorID, policy string, confirmOpen bool) (Policy, error) {
	if !ValidPolicy(policy) {
		return Policy{}, errors.New("invalid_policy")
	}
	if policy == PolicyOpen && !confirmOpen {
		return Policy{}, errors.New("open_confirmation_required")
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	err := store.WithSerializedLock(ctx, s.store, func(tx store.Transaction) error {
		if s.store.Dialect().Provider() == store.Postgres {
			if _, err := tx.ExecContext(ctx, "SELECT id FROM _trestle_app_registration_policy WHERE id=1 FOR UPDATE"); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE _trestle_app_registration_policy SET policy=?,set_at=? WHERE id=1", policy, now); err != nil {
			return err
		}
		if err := s.audit.Emit(ctx, tx, "admin", actorID, "app_registration.policy.change", "", "success", requestID(ctx), map[string]any{"policy": policy}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Policy{}, err
	}
	return Policy{Name: policy, SetAt: now}, nil
}

// Invitation is an application-user activation or self-registration token.
type Invitation struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Email       string  `json:"email"`
	CreatedAt   string  `json:"createdAt"`
	ExpiresAt   string  `json:"expiresAt"`
	UsedAt      *string `json:"usedAt,omitempty"`
	RevokedAt   *string `json:"revokedAt,omitempty"`
	AccessReqID *string `json:"accessRequestId,omitempty"`
	RawToken    string  `json:"token,omitempty"`
}

// CreateInvitation creates an email-bound invitation and returns the raw token
// exactly once. kind is self_register (gated on invite policy at acceptance)
// or activate (valid under every policy, used for provisioning and approval
// outcomes). If userEmail already belongs to an application user, an
// authenticated-admin conflict is returned.
func (s *Service) CreateInvitation(ctx context.Context, adminID, kind, email, accessReqID string) (Invitation, error) {
	email, ok := NormalizeEmail(email)
	if !ok {
		return Invitation{}, errors.New("invalid_email")
	}
	if kind != "self_register" && kind != "activate" {
		return Invitation{}, errors.New("invalid_invitation_kind")
	}
	var count int
	if err := s.store.QueryRowContext(ctx, "SELECT count(*) FROM _trestle_app_users WHERE email=?", email).Scan(&count); err != nil {
		return Invitation{}, err
	}
	if count != 0 {
		return Invitation{}, errors.New("user_already_exists")
	}
	raw, err := s.randomToken(32)
	if err != nil {
		return Invitation{}, err
	}
	var inv Invitation
	err = store.WithTx(ctx, s.store, func(tx store.Transaction) error {
		created, cerr := s.createInvitationTx(ctx, tx, adminID, kind, email, accessReqID, raw)
		if cerr != nil {
			return cerr
		}
		inv = created
		return s.audit.Emit(ctx, tx, "admin", adminID, "app_registration.invitation.create", created.ID, "success", requestID(ctx), map[string]any{"kind": kind, "email": email})
	})
	return inv, err
}

// ListInvitations lists invitations for administrators, never returning raw
// tokens or token hashes.
func (s *Service) ListInvitations(ctx context.Context) ([]Invitation, error) {
	rows, err := s.store.QueryContext(ctx, "SELECT id,kind,email,created_at,expires_at,used_at,revoked_at,access_request_id FROM _trestle_app_invitations ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Invitation{}
	for rows.Next() {
		var it Invitation
		var used, revoked, areq sql.NullString
		if err := rows.Scan(&it.ID, &it.Kind, &it.Email, &it.CreatedAt, &it.ExpiresAt, &used, &revoked, &areq); err != nil {
			return nil, err
		}
		it.UsedAt = nullStringPtr(used)
		it.RevokedAt = nullStringPtr(revoked)
		it.AccessReqID = nullStringPtr(areq)
		items = append(items, it)
	}
	return items, rows.Err()
}

// RevokeInvitation revokes an unused invitation (idempotent) and audits it.
func (s *Service) RevokeInvitation(ctx context.Context, adminID, id string) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	return store.WithTx(ctx, s.store, func(tx store.Transaction) error {
		result, err := tx.ExecContext(ctx, "UPDATE _trestle_app_invitations SET revoked_at=? WHERE id=? AND used_at IS NULL AND revoked_at IS NULL", now, id)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			// Nonexistent, already used or already revoked: idempotent success
			// without a false "successfully revoked" audit fact.
			return nil
		}
		return s.audit.Emit(ctx, tx, "admin", adminID, "app_registration.invitation.revoke", id, "success", requestID(ctx), nil)
	})
}

// AccessRequest is a pending or decided access request.
type AccessRequest struct {
	ID           string  `json:"id"`
	Email        string  `json:"email"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"createdAt"`
	DecidedAt    *string `json:"decidedAt,omitempty"`
	DecidedBy    *string `json:"decidedBy,omitempty"`
	InvitationID *string `json:"invitationId,omitempty"`
}

// SubmitAccessRequest records a bounded pending access request. It acquires
// the serialized policy lock, verifies the policy is still approval inside the
// transaction, performs the bounded expiry/purge sweep and the duplicate/user
// checks, and inserts - so a restrictive policy change that wins the lock first
// prevents the request from being created. A policy-rejection or genuine
// storage error is returned distinctly; the public handler decides how much to
// reveal. Genuine storage failures increment StorageFailures so operators can
// observe them without the caller receiving a different response.
func (s *Service) SubmitAccessRequest(ctx context.Context, email string) error {
	email, ok := NormalizeEmail(email)
	if !ok {
		return errors.New("invalid_email")
	}
	now := s.now().UTC()
	err := store.WithSerializedLock(ctx, s.store, func(tx store.Transaction) error {
		policy, perr := s.policyForTx(ctx, tx)
		if perr != nil {
			return perr
		}
		if policy != PolicyApproval {
			return errors.New("policy_not_approval")
		}
		if err := s.sweepExpired(ctx, tx, now); err != nil {
			return err
		}
		var pending int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM _trestle_app_access_requests WHERE email=? AND status='pending'", email).Scan(&pending); err != nil {
			return err
		}
		var userCount int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM _trestle_app_users WHERE email=?", email).Scan(&userCount); err != nil {
			return err
		}
		if pending > 0 || userCount > 0 {
			return nil
		}
		raw, err := s.randomToken(18)
		if err != nil {
			return err
		}
		id := "areq_" + raw
		_, err = tx.ExecContext(ctx, "INSERT INTO _trestle_app_access_requests(id,email,status,created_at) VALUES(?,?,'pending',?)", id, email, now.Format(time.RFC3339Nano))
		return err
	})
	if err != nil && !strings.HasPrefix(err.Error(), "policy_not_approval") {
		s.failures.Add(1)
	}
	return err
}

// policyForTx locks and reads the singleton policy row on the caller's
// transaction. On PostgreSQL the read uses SELECT ... FOR UPDATE; on SQLite the
// serialized transaction already holds the write lock.
func (s *Service) policyForTx(ctx context.Context, tx store.Transaction) (string, error) {
	if s.store.Dialect().Provider() == store.Postgres {
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

// ListAccessRequests lists requests for administrators, distinguishing
// pending/approved/rejected/expired, and runs a bounded sweep.
func (s *Service) ListAccessRequests(ctx context.Context) ([]AccessRequest, error) {
	if err := s.sweepExpired(ctx, s.store, s.now().UTC()); err != nil {
		return nil, err
	}
	rows, err := s.store.QueryContext(ctx, "SELECT id,email,status,created_at,decided_at,decided_by_admin_id FROM _trestle_app_access_requests ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AccessRequest{}
	for rows.Next() {
		var a AccessRequest
		var decided, by sql.NullString
		if err := rows.Scan(&a.ID, &a.Email, &a.Status, &a.CreatedAt, &decided, &by); err != nil {
			return nil, err
		}
		a.DecidedAt = nullStringPtr(decided)
		a.DecidedBy = nullStringPtr(by)
		items = append(items, a)
	}
	return items, rows.Err()
}

// ApproveRequest approves a pending request exactly once, creating an activate
// invitation (with access_request_id) and auditing, all in one transaction.
// The request row is locked (PostgreSQL SELECT ... FOR UPDATE) and the
// conditional update must affect exactly one row, so two concurrent approvals
// yield exactly one winner and one activation invitation. Returns the new raw
// activation token exactly once.
func (s *Service) ApproveRequest(ctx context.Context, adminID, id string) (Invitation, error) {
	decided := s.now().UTC().Format(time.RFC3339Nano)
	var inv Invitation
	err := store.WithTx(ctx, s.store, func(tx store.Transaction) error {
		var email string
		var status string
		if s.store.Dialect().Provider() == store.Postgres {
			if err := tx.QueryRowContext(ctx, "SELECT email,status FROM _trestle_app_access_requests WHERE id=? FOR UPDATE", id).Scan(&email, &status); err != nil {
				return errors.New("already_decided")
			}
		} else {
			if err := tx.QueryRowContext(ctx, "SELECT email,status FROM _trestle_app_access_requests WHERE id=?", id).Scan(&email, &status); err != nil {
				return errors.New("already_decided")
			}
		}
		if status != "pending" {
			return errors.New("already_decided")
		}
		result, err := tx.ExecContext(ctx, "UPDATE _trestle_app_access_requests SET status='approved',decided_at=?,decided_by_admin_id=? WHERE id=? AND status='pending'", decided, adminID, id)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return errors.New("already_decided")
		}
		raw, rerr := s.randomToken(32)
		if rerr != nil {
			return rerr
		}
		created, err := s.createInvitationTx(ctx, tx, adminID, "activate", email, id, raw)
		if err != nil {
			return err
		}
		inv = created
		return s.audit.Emit(ctx, tx, "admin", adminID, "app_registration.request.approve", id, "success", requestID(ctx), map[string]any{"email": email})
	})
	if err != nil {
		return Invitation{}, err
	}
	return inv, nil
}

// RejectRequest rejects a pending request exactly once and audits it. The
// conditional update must affect exactly one row, so concurrent decisions have
// one winner. No user or invitation is created.
func (s *Service) RejectRequest(ctx context.Context, adminID, id string) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	return store.WithTx(ctx, s.store, func(tx store.Transaction) error {
		result, err := tx.ExecContext(ctx, "UPDATE _trestle_app_access_requests SET status='rejected',decided_at=?,decided_by_admin_id=? WHERE id=? AND status='pending'", now, adminID, id)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return errors.New("already_decided")
		}
		return s.audit.Emit(ctx, tx, "admin", adminID, "app_registration.request.reject", id, "success", requestID(ctx), nil)
	})
}

// ReissueInvitation issues a replacement activate invitation for an approved
// request, deriving the email exclusively from the locked request row. The
// caller cannot substitute a different email. It revokes genuinely outstanding
// activation invitations for that email in the same transaction, creates
// exactly one replacement, and audits the reissue. The request stays approved.
func (s *Service) ReissueInvitation(ctx context.Context, adminID, accessReqID string) (Invitation, error) {
	now := s.now().UTC().Format(time.RFC3339Nano)
	raw, rerr := s.randomToken(32)
	if rerr != nil {
		return Invitation{}, rerr
	}
	sum := sha256.Sum256([]byte(raw))
	id := "inv_" + now // placeholder replaced below
	_ = id
	expires := s.now().UTC().Add(InvitationLifetime).Format(time.RFC3339Nano)
	var inv Invitation
	err := store.WithTx(ctx, s.store, func(tx store.Transaction) error {
		var email, status string
		if s.store.Dialect().Provider() == store.Postgres {
			if err := tx.QueryRowContext(ctx, "SELECT email,status FROM _trestle_app_access_requests WHERE id=? FOR UPDATE", accessReqID).Scan(&email, &status); err != nil {
				return errors.New("request_not_found")
			}
		} else {
			if err := tx.QueryRowContext(ctx, "SELECT email,status FROM _trestle_app_access_requests WHERE id=?", accessReqID).Scan(&email, &status); err != nil {
				return errors.New("request_not_found")
			}
		}
		if status != "approved" {
			return errors.New("request_not_approved")
		}
		var userCount int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM _trestle_app_users WHERE email=?", email).Scan(&userCount); err != nil {
			return err
		}
		if userCount != 0 {
			return errors.New("user_already_exists")
		}
		if _, err := tx.ExecContext(ctx, "UPDATE _trestle_app_invitations SET revoked_at=? WHERE email=? AND kind='activate' AND used_at IS NULL AND revoked_at IS NULL", now, email); err != nil {
			return err
		}
		idTok, idErr := s.randomToken(18)
		if idErr != nil {
			return idErr
		}
		id = "inv_" + idTok
		if _, err := tx.ExecContext(ctx, "INSERT INTO _trestle_app_invitations(id,kind,email,token_hash,created_at,expires_at,created_by_admin_id,access_request_id) VALUES(?,?,?,?,?,?,?,?)",
			id, "activate", email, sum[:], now, expires, adminID, accessReqID); err != nil {
			return err
		}
		inv = Invitation{ID: id, Kind: "activate", Email: email, CreatedAt: now, ExpiresAt: expires, RawToken: raw, AccessReqID: &accessReqID}
		return s.audit.Emit(ctx, tx, "admin", adminID, "app_registration.invitation.reissue", id, "success", requestID(ctx), map[string]any{"email": email})
	})
	if err != nil {
		return Invitation{}, err
	}
	return inv, nil
}

// sweepExpired transitions expired pending requests to the terminal `expired`
// state and purges expired/rejected requests beyond the retention window, in
// bounded batches of at most CleanupBatch rows per trigger with deterministic
// ordering. It runs only during access-request-related operations, so a single
// public request never drains the whole backlog.
type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// sweepExpired transitions expired pending requests to the terminal `expired`
// state and purges expired/rejected requests beyond the retention window, in
// bounded batches of at most CleanupBatch rows per trigger with deterministic
// ordering. It runs only during access-request-related operations, so a single
// public request never drains the whole backlog.
func (s *Service) sweepExpired(ctx context.Context, e execer, now time.Time) error {
	expiredCutoff := now.Add(-AccessRequestPendingTTL).Format(time.RFC3339Nano)
	retentionCutoff := now.Add(-AccessRequestRetentionTTL).Format(time.RFC3339Nano)
	if _, err := e.ExecContext(ctx, "UPDATE _trestle_app_access_requests SET status='expired' WHERE id IN (SELECT id FROM _trestle_app_access_requests WHERE status='pending' AND created_at <= ? ORDER BY id LIMIT ?)", expiredCutoff, CleanupBatch); err != nil {
		return err
	}
	if _, err := e.ExecContext(ctx, "DELETE FROM _trestle_app_access_requests WHERE id IN (SELECT id FROM _trestle_app_access_requests WHERE status IN ('expired','rejected') AND (decided_at IS NOT NULL AND decided_at <= ? OR decided_at IS NULL AND created_at <= ?) ORDER BY id LIMIT ?)", retentionCutoff, retentionCutoff, CleanupBatch); err != nil {
		return err
	}
	return nil
}

func (s *Service) createInvitationTx(ctx context.Context, tx store.Transaction, adminID, kind, email, accessReqID, raw string) (Invitation, error) {
	id, err := s.randomToken(18)
	if err != nil {
		return Invitation{}, err
	}
	id = "inv_" + id
	sum := sha256.Sum256([]byte(raw))
	now := s.now().UTC().Format(time.RFC3339Nano)
	expires := s.now().UTC().Add(InvitationLifetime).Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, "INSERT INTO _trestle_app_invitations(id,kind,email,token_hash,created_at,expires_at,created_by_admin_id,access_request_id) VALUES(?,?,?,?,?,?,?,?)",
		id, kind, email, sum[:], now, expires, adminID, nullStr(accessReqID)); err != nil {
		return Invitation{}, err
	}
	return Invitation{ID: id, Kind: kind, Email: email, CreatedAt: now, ExpiresAt: expires, RawToken: raw}, nil
}

// NormalizeEmail lowercases and validates an email address.
func NormalizeEmail(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	return value, err == nil && parsed.Address == value && len(value) <= 254
}

func (s *Service) randomToken(n int) (string, error) {
	b := make([]byte, n)
	read, err := s.randReader(b)
	if err != nil || read != n {
		return "", errors.New("entropy_source_unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		v := ns.String
		return &v
	}
	return nil
}

func requestID(ctx context.Context) string {
	if v, ok := ctx.Value("requestID").(string); ok {
		return v
	}
	return ""
}

// ActivationBaseURL is the optional administrator-configured application URL
// used to build fragment-only activation links. It is stored in system_meta
// key app_activation_base_url.
func (s *Service) ActivationBaseURL(ctx context.Context) (string, error) {
	var value string
	err := s.store.QueryRowContext(ctx, "SELECT value FROM _trestle_system_meta WHERE key='app_activation_base_url'").Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// SetActivationBaseURL validates, normalizes and stores the activation base
// URL, returning the canonical serialized value that was persisted (or "" when
// cleared). It must be absolute, https (or http for loopback hosts in local
// development), free of username/password, fragments and query strings, bounded
// in length, and free of control characters; surrounding whitespace is trimmed
// and the canonical serialization is stored so the value returned to clients
// always matches what the link builder uses.
func (s *Service) SetActivationBaseURL(ctx context.Context, adminID, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		err := store.WithTx(ctx, s.store, func(tx store.Transaction) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM _trestle_system_meta WHERE key='app_activation_base_url'"); err != nil {
				return err
			}
			return s.audit.Emit(ctx, tx, "admin", adminID, "app_registration.activation_base_url.clear", "", "success", requestID(ctx), nil)
		})
		return "", err
	}
	parsed, err := ValidateActivationBaseURL(value)
	if err != nil {
		return "", err
	}
	canonical := parsed.String()
	now := s.now().UTC().Format(time.RFC3339Nano)
	err = store.WithTx(ctx, s.store, func(tx store.Transaction) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO _trestle_system_meta(key,value,updated_at) VALUES('app_activation_base_url',?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at", canonical, now); err != nil {
			return err
		}
		return s.audit.Emit(ctx, tx, "admin", adminID, "app_registration.activation_base_url.set", "", "success", requestID(ctx), nil)
	})
	return canonical, err
}

// ActivationLink builds a fragment-only activation link from the configured
// base URL and a raw token, using URL construction rather than string
// concatenation. The token never appears in the path, query or fragment of an
// HTTP request target; it is placed in the fragment so the browser reads it
// client-side and submits it in the POST body.
func ActivationLink(baseURL, rawToken string) (string, error) {
	parsed, err := ValidateActivationBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	parsed.Fragment = "invite=" + rawToken
	return parsed.String(), nil
}

// ValidateActivationBaseURL validates and normalizes an activation base URL.
func ValidateActivationBaseURL(value string) (*url.URL, error) {
	value = strings.TrimSpace(value)
	if len(value) > 2048 {
		return nil, errors.New("activation_base_url_too_long")
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 0x20 || value[i] == 0x7f {
			return nil, errors.New("activation_base_url_invalid")
		}
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return nil, errors.New("activation_base_url_invalid")
	}
	if parsed.User != nil {
		return nil, errors.New("activation_base_url_invalid")
	}
	if parsed.Fragment != "" || parsed.RawQuery != "" {
		return nil, errors.New("activation_base_url_invalid")
	}
	if parsed.Scheme != "https" {
		host := parsed.Hostname()
		if parsed.Scheme != "http" || !(host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1") {
			return nil, errors.New("activation_base_url_invalid")
		}
	}
	return parsed, nil
}

// AuditUserCreation records a successful application-user creation. The audit
// actor contract distinguishes public registration from administrator
// activation:
//
//   - actorKind "system", actorID "public-registration" for open registration
//     and self-register invitation acceptance;
//   - actorKind "system", actorID "activation" for administrator-activation
//     invitation acceptance.
//
// The created user ID is the audit target, so the durable record is
// identifiable. The creation path stays in bounded details; passwords and raw
// tokens are never included. Audit is a mandatory dependency: account creation
// fails closed (returns an error, rolling back the user) when the audit handler
// has not been wired, rather than silently creating a user without the promised
// audit fact.
func (s *Service) AuditUserCreation(ctx context.Context, tx store.Transaction, userID, email, path string) error {
	if s.audit == nil {
		return errors.New("audit_not_configured")
	}
	actor := "public-registration"
	if path == "activate" {
		actor = "activation"
	}
	return s.audit.Emit(ctx, tx, "system", actor, "app_registration.user.create", userID, "success", requestID(ctx), map[string]any{"email": email, "path": path})
}

// SetRandomReader overrides the entropy source (test injection).
func (s *Service) SetRandomReader(fn func([]byte) (int, error)) { s.randReader = fn }

// SetStore overrides the store executor (test injection).
func (s *Service) SetStore(e store.Executor) { s.store = e }
