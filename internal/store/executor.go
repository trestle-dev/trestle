package store

import (
	"context"
	"database/sql"
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

type postgresDialect struct{}

func (postgresDialect) Provider() Provider                { return Postgres }
func (postgresDialect) Bind(query string) (string, error) { return RebindPostgres(query) }
func (postgresDialect) Boolean(value bool) any            { return value }
func NewDialect(provider Provider) Dialect {
	if provider == Postgres {
		return postgresDialect{}
	}
	return sqliteDialect{}
}
