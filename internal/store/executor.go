package store

import (
	"context"
	"database/sql"
	"fmt"
)

type Executor interface {
	Exec(string, ...any) (sql.Result, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
	QueryRowContext(context.Context, string, ...any) *sql.Row
	Begin() (Transaction, error)
	BeginTx(context.Context, *sql.TxOptions) (Transaction, error)
	Dialect() Dialect
}

type Transaction interface {
	Exec(string, ...any) (sql.Result, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
	QueryRowContext(context.Context, string, ...any) *sql.Row
	Commit() error
	Rollback() error
}

type boundExecutor struct {
	db      *sql.DB
	dialect Dialect
}
type boundTransaction struct {
	tx      *sql.Tx
	dialect Dialect
}

func Adapt(value any) Executor {
	if executor, ok := value.(Executor); ok {
		return executor
	}
	if db, ok := value.(*sql.DB); ok {
		return NewExecutor(db, NewDialect(SQLite))
	}
	panic("store executor must be *sql.DB or store.Executor")
}
func NewExecutor(db *sql.DB, dialect Dialect) Executor {
	return &boundExecutor{db: db, dialect: dialect}
}

func NewExecutorTx(tx *sql.Tx, dialect Dialect) Transaction {
	return &boundTransaction{tx: tx, dialect: dialect}
}
func (e *boundExecutor) Exec(q string, a ...any) (sql.Result, error) {
	return e.db.Exec(Bind(e.dialect, q), a...)
}
func (e *boundExecutor) ExecContext(c context.Context, q string, a ...any) (sql.Result, error) {
	return e.db.ExecContext(c, Bind(e.dialect, q), a...)
}
func (e *boundExecutor) Query(q string, a ...any) (*sql.Rows, error) {
	return e.db.Query(Bind(e.dialect, q), a...)
}
func (e *boundExecutor) QueryContext(c context.Context, q string, a ...any) (*sql.Rows, error) {
	return e.db.QueryContext(c, Bind(e.dialect, q), a...)
}
func (e *boundExecutor) QueryRow(q string, a ...any) *sql.Row {
	return e.db.QueryRow(Bind(e.dialect, q), a...)
}
func (e *boundExecutor) QueryRowContext(c context.Context, q string, a ...any) *sql.Row {
	return e.db.QueryRowContext(c, Bind(e.dialect, q), a...)
}
func (e *boundExecutor) Begin() (Transaction, error) {
	tx, err := e.db.Begin()
	if err != nil {
		return nil, err
	}
	return &boundTransaction{tx: tx, dialect: e.dialect}, nil
}
func (e *boundExecutor) Dialect() Dialect { return e.dialect }
func (e *boundExecutor) BeginTx(c context.Context, o *sql.TxOptions) (Transaction, error) {
	tx, err := e.db.BeginTx(c, o)
	if err != nil {
		return nil, err
	}
	return &boundTransaction{tx: tx, dialect: e.dialect}, nil
}
func (t *boundTransaction) Exec(q string, a ...any) (sql.Result, error) {
	return t.tx.Exec(Bind(t.dialect, q), a...)
}
func (t *boundTransaction) ExecContext(c context.Context, q string, a ...any) (sql.Result, error) {
	return t.tx.ExecContext(c, Bind(t.dialect, q), a...)
}
func (t *boundTransaction) Query(q string, a ...any) (*sql.Rows, error) {
	return t.tx.Query(Bind(t.dialect, q), a...)
}
func (t *boundTransaction) QueryContext(c context.Context, q string, a ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(c, Bind(t.dialect, q), a...)
}
func (t *boundTransaction) QueryRow(q string, a ...any) *sql.Row {
	return t.tx.QueryRow(Bind(t.dialect, q), a...)
}
func (t *boundTransaction) QueryRowContext(c context.Context, q string, a ...any) *sql.Row {
	return t.tx.QueryRowContext(c, Bind(t.dialect, q), a...)
}
func (t *boundTransaction) Commit() error   { return t.tx.Commit() }
func (t *boundTransaction) Rollback() error { return t.tx.Rollback() }

type Dialect interface {
	Provider() Provider
	Bind(string) (string, error)
	Boolean(bool) any
	DecodeBoolean(any) (bool, error)
	ColumnType(kind string) (string, error)
	BooleanCheck(column string) string
	JSONCheck(column string) string
	NumberCheck(column string) string
	TableSuffix() string
	ContainsOperator() string
}
type sqliteDialect struct{}

func (sqliteDialect) Provider() Provider                { return SQLite }
func (sqliteDialect) Bind(query string) (string, error) { return query, nil }
func (sqliteDialect) Boolean(value bool) any {
	if value {
		return 1
	}
	return 0
}
func (sqliteDialect) DecodeBoolean(value any) (bool, error) {
	switch v := value.(type) {
	case int64:
		return v == 1, nil
	case bool:
		return v, nil
	default:
		return false, fmt.Errorf("unsupported boolean value %T", value)
	}
}
func (sqliteDialect) ColumnType(kind string) (string, error) {
	switch kind {
	case "text", "email", "url", "select", "relation", "datetime", "json":
		return "TEXT", nil
	case "number":
		return "REAL", nil
	case "boolean":
		return "INTEGER", nil
	default:
		return "", fmt.Errorf("unsupported field type %q", kind)
	}
}
func (sqliteDialect) BooleanCheck(column string) string {
	return fmt.Sprintf(" CHECK(%s IN (0,1))", column)
}
func (sqliteDialect) JSONCheck(column string) string {
	return fmt.Sprintf(" CHECK(json_valid(%s))", column)
}
func (sqliteDialect) NumberCheck(column string) string {
	return fmt.Sprintf(" CHECK(typeof(%s) IN ('real','integer','null'))", column)
}
func (sqliteDialect) TableSuffix() string { return " STRICT" }
func (sqliteDialect) ContainsOperator() string {
	// SQLite's default ASCII LIKE is case-insensitive, so the contains
	// operator matches that behavior directly.
	return "LIKE"
}

type postgresDialect struct{}

func (postgresDialect) Provider() Provider                { return Postgres }
func (postgresDialect) Bind(query string) (string, error) { return RebindPostgres(query) }
func (postgresDialect) Boolean(value bool) any            { return value }
func (postgresDialect) DecodeBoolean(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case int64:
		return v == 1, nil
	default:
		return false, fmt.Errorf("unsupported boolean value %T", value)
	}
}
func (postgresDialect) ColumnType(kind string) (string, error) {
	switch kind {
	case "text", "email", "url", "select", "relation", "datetime", "json":
		return "TEXT", nil
	case "number":
		return "DOUBLE PRECISION", nil
	case "boolean":
		return "BOOLEAN", nil
	default:
		return "", fmt.Errorf("unsupported field type %q", kind)
	}
}
func (postgresDialect) BooleanCheck(column string) string { return "" }
func (postgresDialect) JSONCheck(column string) string {
	// Keep JSON as TEXT but enforce validity at the database level so an
	// invalid value written outside the HTTP validator is rejected exactly as
	// SQLite's json_valid check rejects it. SQL NULL (unset) is allowed; the
	// JSON literal "null" is valid text that still passes the cast.
	return fmt.Sprintf(" CHECK(%s IS NULL OR (%s::jsonb) IS NOT NULL)", column, column)
}
func (postgresDialect) NumberCheck(column string) string { return "" }
func (postgresDialect) TableSuffix() string              { return "" }
func (postgresDialect) ContainsOperator() string {
	// PostgreSQL LIKE is case-sensitive, which would diverge from SQLite's
	// case-insensitive contains behavior. ILIKE restores parity.
	return "ILIKE"
}
func NewDialect(provider Provider) Dialect {
	if provider == Postgres {
		return postgresDialect{}
	}
	return sqliteDialect{}
}

// WithTx runs fn inside a normal writable transaction and commits on success,
// rolling back on error.
func WithTx(ctx context.Context, e Executor, fn func(tx Transaction) error) error {
	tx, err := e.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
