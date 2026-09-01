package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Application registration policy names and the migration that introduces the
// registration-policy schema.
const (
	registrationMigrationVersion = 15

	PolicyOpen     = "open"
	PolicyInvite   = "invite"
	PolicyApproval = "approval"
	PolicyClosed   = "closed"
)

// seedRegistrationPolicy records the initial application registration policy
// during migration 15. A genuinely new database (startingSchemaVersion == 0)
// starts closed and requires an explicit first-run policy selection; an
// existing pre-v15 deployment starts open, regardless of user count, because
// the pre-v15 era had open registration. The decision is recorded once here
// from durable migration state and is never re-derived from row counts.
func seedRegistrationPolicy(ctx context.Context, tx Transaction, startingSchemaVersion int, now string) error {
	initial := PolicyClosed
	if startingSchemaVersion > 0 {
		initial = PolicyOpen
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO _trestle_app_registration_policy(id,policy,set_at) VALUES(1,?,?) ON CONFLICT(id) DO NOTHING", initial, now); err != nil {
		return err
	}
	return nil
}

// SerializedBeginner is implemented by executors that can begin a transaction
// holding the database write lock from its first statement (the registration
// policy serialization primitive).
type SerializedBeginner interface {
	BeginSerialized(ctx context.Context) (Transaction, error)
}

// BeginSerialized begins a transaction that holds the database write lock for
// its whole lifetime:
//
//   - SQLite: BEGIN IMMEDIATE on a dedicated connection, so the write lock is
//     held before any policy read and a concurrent policy change cannot
//     interleave between the policy read and the mutation's commit.
//   - PostgreSQL: a normal writable transaction; callers serialize on the
//     singleton policy row with SELECT ... FOR UPDATE.
func (e *boundExecutor) BeginSerialized(ctx context.Context) (Transaction, error) {
	if e.dialect.Provider() == Postgres {
		tx, err := e.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		return NewExecutorTx(tx, e.dialect), nil
	}
	conn, err := e.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &serialConnTx{conn: conn, dialect: e.dialect}, nil
}

// serialConnTx runs every statement on a dedicated connection inside a
// BEGIN IMMEDIATE transaction and always returns the connection to the pool.
type serialConnTx struct {
	conn    *sql.Conn
	dialect Dialect
	done    bool
}

func (t *serialConnTx) Exec(q string, a ...any) (sql.Result, error) {
	return t.conn.ExecContext(context.Background(), Bind(t.dialect, q), a...)
}
func (t *serialConnTx) ExecContext(ctx context.Context, q string, a ...any) (sql.Result, error) {
	return t.conn.ExecContext(ctx, Bind(t.dialect, q), a...)
}
func (t *serialConnTx) Query(q string, a ...any) (*sql.Rows, error) {
	return t.conn.QueryContext(context.Background(), Bind(t.dialect, q), a...)
}
func (t *serialConnTx) QueryContext(ctx context.Context, q string, a ...any) (*sql.Rows, error) {
	return t.conn.QueryContext(ctx, Bind(t.dialect, q), a...)
}
func (t *serialConnTx) QueryRow(q string, a ...any) *sql.Row {
	return t.conn.QueryRowContext(context.Background(), Bind(t.dialect, q), a...)
}
func (t *serialConnTx) QueryRowContext(ctx context.Context, q string, a ...any) *sql.Row {
	return t.conn.QueryRowContext(ctx, Bind(t.dialect, q), a...)
}

// Commit ends the transaction exactly once and always returns the connection
// to the pool, even when the COMMIT fails.
func (t *serialConnTx) Commit() error {
	if t.done {
		return errors.New("serialized transaction already closed")
	}
	t.done = true
	_, err := t.conn.ExecContext(context.Background(), "COMMIT")
	_ = t.conn.Close()
	return err
}

// Rollback aborts the transaction exactly once and always returns the
// connection to the pool.
func (t *serialConnTx) Rollback() error {
	if t.done {
		return errors.New("serialized transaction already closed")
	}
	t.done = true
	_, err := t.conn.ExecContext(context.Background(), "ROLLBACK")
	_ = t.conn.Close()
	return err
}

// IsLockBusy reports whether err is a transient lock/serialization failure the
// caller may retry: SQLite SQLITE_BUSY/SQLITE_LOCKED and PostgreSQL
// serialization_failure (40001) / deadlock_detected (40P01).
func IsLockBusy(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "sqlite_busy") || strings.Contains(lower, "sqlite_locked") ||
		strings.Contains(lower, "database is locked") || strings.Contains(lower, "database table is locked") {
		return true
	}
	// lib/pq surfaces PostgreSQL SQLSTATE as the "pq: ..." message; match the
	// standard SQLSTATE codes embedded in the driver error text.
	if strings.Contains(err.Error(), "40001") || strings.Contains(err.Error(), "40P01") {
		return true
	}
	return false
}

// backoff sleeps for the given attempt with jitter, bounded below one second.
func backoff(ctx context.Context, attempt int) {
	if attempt <= 0 {
		return
	}
	delay := time.Duration(10*(1<<uint(attempt-1))) * time.Millisecond // 10,20,40,80
	if delay > 200*time.Millisecond {
		delay = 200 * time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// WithSerializedLock runs fn inside a serialized transaction, retrying bounded
// times on transient lock/serialization failures. On exhaustion it returns an
// error wrapping ErrLockExhausted so callers can map it to
// registration_temporarily_unavailable rather than a 4xx application error.
var ErrLockExhausted = errors.New("registration policy lock exhausted")

const maxSerializedAttempts = 5

func WithSerializedLock(ctx context.Context, e Executor, fn func(tx Transaction) error) error {
	beginner, ok := e.(SerializedBeginner)
	if !ok {
		return errors.New("executor does not support serialized transactions")
	}
	var lastErr error
	for attempt := 0; attempt < maxSerializedAttempts; attempt++ {
		tx, err := beginner.BeginSerialized(ctx)
		if err != nil {
			if IsLockBusy(err) {
				backoff(ctx, attempt+1)
				lastErr = err
				continue
			}
			return err
		}
		runErr := fn(tx)
		if runErr == nil {
			if cerr := tx.Commit(); cerr == nil {
				return nil
			} else if IsLockBusy(cerr) {
				_ = tx.Rollback()
				backoff(ctx, attempt+1)
				lastErr = cerr
				continue
			} else {
				return fmt.Errorf("commit serialized transaction: %w", cerr)
			}
		}
		_ = tx.Rollback()
		if IsLockBusy(runErr) {
			lastErr = runErr
			backoff(ctx, attempt+1)
			continue
		}
		return runErr
	}
	return fmt.Errorf("%w: %v", ErrLockExhausted, lastErr)
}
