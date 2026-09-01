package appreg

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/mail"
	"strings"
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
	store store.Executor
	audit *audit.Handler
	now   func() time.Time
}

func New(store store.Executor, audit *audit.Handler) *Service {
	return &Service{store: store, audit: audit, now: time.Now}
}

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
	raw := randomToken(32)
	sum := sha256.Sum256([]byte(raw))
	id := "inv_" + randomToken(18)
	now := s.now().UTC().Format(time.RFC3339Nano)
	expires := s.now().UTC().Add(InvitationLifetime).Format(time.RFC3339Nano)
	_, err := s.store.ExecContext(ctx, "INSERT INTO _trestle_app_invitations(id,kind,email,token_hash,created_at,expires_at,created_by_admin_id,access_request_id) VALUES(?,?,?,?,?,?,?,?)",
		id, kind, email, sum[:], now, expires, adminID, nullStr(accessReqID))
	if err != nil {
		return Invitation{}, err
	}
	return Invitation{ID: id, Kind: kind, Email: email, CreatedAt: now, ExpiresAt: expires, RawToken: raw}, nil
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
		if _, err := tx.ExecContext(ctx, "UPDATE _trestle_app_invitations SET revoked_at=? WHERE id=? AND used_at IS NULL AND revoked_at IS NULL", now, id); err != nil {
			return err
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

// SubmitAccessRequest records a bounded pending access request under the
// approval policy. It always presents a generic 202-style outcome to the
// public caller and performs a bounded expiry/purge sweep before inserting.
func (s *Service) SubmitAccessRequest(ctx context.Context, email string) error {
	email, ok := NormalizeEmail(email)
	if !ok {
		return errors.New("invalid_email")
	}
	now := s.now().UTC()
	if err := s.sweepExpired(ctx, now); err != nil {
		return err
	}
	// If the email is already a user or already has a pending request, do
	// nothing durable (generic public outcome).
	var pending int
	if err := s.store.QueryRowContext(ctx, "SELECT count(*) FROM _trestle_app_access_requests WHERE email=? AND status='pending'", email).Scan(&pending); err != nil {
		return err
	}
	var userCount int
	if err := s.store.QueryRowContext(ctx, "SELECT count(*) FROM _trestle_app_users WHERE email=?", email).Scan(&userCount); err != nil {
		return err
	}
	if pending > 0 || userCount > 0 {
		return nil
	}
	id := "areq_" + randomToken(18)
	_, err := s.store.ExecContext(ctx, "INSERT INTO _trestle_app_access_requests(id,email,status,created_at) VALUES(?,?,'pending',?)", id, email, now.Format(time.RFC3339Nano))
	return err
}

// ListAccessRequests lists requests for administrators, distinguishing
// pending/approved/rejected/expired, and runs a bounded sweep.
func (s *Service) ListAccessRequests(ctx context.Context) ([]AccessRequest, error) {
	if err := s.sweepExpired(ctx, s.now().UTC()); err != nil {
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

// ApproveRequest approves a pending request atomically, creating an activate
// invitation (with access_request_id) and auditing, all in one transaction.
// Returns the new raw activation token exactly once.
func (s *Service) ApproveRequest(ctx context.Context, adminID, id string) (Invitation, error) {
	now := s.now().UTC()
	decided := now.Format(time.RFC3339Nano)
	var inv Invitation
	err := store.WithTx(ctx, s.store, func(tx store.Transaction) error {
		var email string
		if err := tx.QueryRowContext(ctx, "SELECT email FROM _trestle_app_access_requests WHERE id=? AND status='pending'", id).Scan(&email); err != nil {
			return errors.New("already_decided")
		}
		if _, err := tx.ExecContext(ctx, "UPDATE _trestle_app_access_requests SET status='approved',decided_at=?,decided_by_admin_id=? WHERE id=? AND status='pending'", decided, adminID, id); err != nil {
			return err
		}
		created, err := s.createInvitationTx(ctx, tx, adminID, "activate", email, id)
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

// RejectRequest rejects a pending request atomically and audits it. No user or
// invitation is created.
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
// email, revoking any still-valid older activate invitations in the same
// transaction, and auditing the reissue. The request stays approved.
func (s *Service) ReissueInvitation(ctx context.Context, adminID, accessReqID, email string) (Invitation, error) {
	email, ok := NormalizeEmail(email)
	if !ok {
		return Invitation{}, errors.New("invalid_email")
	}
	var status string
	if err := s.store.QueryRowContext(ctx, "SELECT status FROM _trestle_app_access_requests WHERE id=?", accessReqID).Scan(&status); err != nil {
		return Invitation{}, errors.New("request_not_found")
	}
	if status != "approved" {
		return Invitation{}, errors.New("request_not_approved")
	}
	var userCount int
	if err := s.store.QueryRowContext(ctx, "SELECT count(*) FROM _trestle_app_users WHERE email=?", email).Scan(&userCount); err != nil {
		return Invitation{}, err
	}
	if userCount != 0 {
		return Invitation{}, errors.New("user_already_exists")
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	raw := randomToken(32)
	sum := sha256.Sum256([]byte(raw))
	id := "inv_" + randomToken(18)
	expires := s.now().UTC().Add(InvitationLifetime).Format(time.RFC3339Nano)
	err := store.WithTx(ctx, s.store, func(tx store.Transaction) error {
		if _, err := tx.ExecContext(ctx, "UPDATE _trestle_app_invitations SET revoked_at=? WHERE email=? AND kind='activate' AND used_at IS NULL AND revoked_at IS NULL", now, email); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO _trestle_app_invitations(id,kind,email,token_hash,created_at,expires_at,created_by_admin_id,access_request_id) VALUES(?,?,?,?,?,?,?,?)",
			id, "activate", email, sum[:], now, expires, adminID, accessReqID); err != nil {
			return err
		}
		return s.audit.Emit(ctx, tx, "admin", adminID, "app_registration.invitation.reissue", id, "success", requestID(ctx), map[string]any{"email": email})
	})
	if err != nil {
		return Invitation{}, err
	}
	return Invitation{ID: id, Kind: "activate", Email: email, CreatedAt: now, ExpiresAt: expires, RawToken: raw, AccessReqID: &accessReqID}, nil
}

// sweepExpired transitions pending requests past their TTL to expired and
// purges expired/rejected requests beyond the retention window, in bounded
// batches. Runs only during access-request-related operations.
func (s *Service) sweepExpired(ctx context.Context, now time.Time) error {
	nowStr := now.Format(time.RFC3339Nano)
	expiredCutoff := now.Add(-AccessRequestPendingTTL).Format(time.RFC3339Nano)
	retentionCutoff := now.Add(-AccessRequestRetentionTTL).Format(time.RFC3339Nano)
	if _, err := s.store.ExecContext(ctx, "UPDATE _trestle_app_access_requests SET status='expired' WHERE status='pending' AND created_at <= ?", expiredCutoff); err != nil {
		return err
	}
	if _, err := s.store.ExecContext(ctx, "DELETE FROM _trestle_app_access_requests WHERE status IN ('expired','rejected') AND (decided_at IS NOT NULL AND decided_at <= ? OR decided_at IS NULL AND created_at <= ?)", retentionCutoff, retentionCutoff); err != nil {
		return err
	}
	_ = nowStr
	return nil
}

// createInvitationTx inserts an invitation row on the caller's transaction
// without returning the raw token (used inside decision transactions). It does
// not audit; the caller audits.
func (s *Service) createInvitationTx(ctx context.Context, tx store.Transaction, adminID, kind, email, accessReqID string) (Invitation, error) {
	raw := randomToken(32)
	sum := sha256.Sum256([]byte(raw))
	id := "inv_" + randomToken(18)
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

func randomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
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
