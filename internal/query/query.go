package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/trestle-dev/trestle/internal/store"
)

const MaxExpressionBytes = 512
const MaxClauses = 8

var clausePattern = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*(=|!=|>=|<=|>|<|~)\s*(.+)$`)

type Field struct {
	Column string
	Type   string
}

type Clause struct {
	Field string
	Op    string
	Value any
}

type Expr struct{ Clauses []Clause }

func Parse(input string) (Expr, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Expr{}, nil
	}
	if len(input) > MaxExpressionBytes {
		return Expr{}, errors.New("filter is too long")
	}
	parts := strings.Split(input, "&&")
	if len(parts) > MaxClauses {
		return Expr{}, errors.New("filter has too many clauses")
	}
	expr := Expr{Clauses: make([]Clause, 0, len(parts))}
	for _, part := range parts {
		match := clausePattern.FindStringSubmatch(strings.TrimSpace(part))
		if match == nil {
			return Expr{}, errors.New("invalid filter clause")
		}
		var value any
		if err := json.Unmarshal([]byte(match[3]), &value); err != nil {
			return Expr{}, errors.New("filter values must be JSON literals")
		}
		expr.Clauses = append(expr.Clauses, Clause{Field: match[1], Op: match[2], Value: value})
	}
	return expr, nil
}

func Compile(expr Expr, fields map[string]Field, dialect store.Dialect) (string, []any, error) {
	parts := make([]string, 0, len(expr.Clauses))
	args := make([]any, 0, len(expr.Clauses))
	for _, clause := range expr.Clauses {
		field, ok := fields[clause.Field]
		if !ok {
			return "", nil, fmt.Errorf("unknown field %q", clause.Field)
		}
		if !compatible(field.Type, clause.Op, clause.Value) {
			return "", nil, fmt.Errorf("operator or value is invalid for %q", clause.Field)
		}
		op := clause.Op
		value := clause.Value
		if value == nil {
			if op == "=" {
				parts = append(parts, quote(field.Column)+" IS NULL")
			} else {
				parts = append(parts, quote(field.Column)+" IS NOT NULL")
			}
			continue
		}
		if op == "~" {
			// Case-insensitive substring match with frozen semantics: the
			// dialect owns the operator (LIKE on SQLite, ILIKE on PostgreSQL)
			// and % and _ act as SQL wildcards. Escaping is not supported.
			parts = append(parts, quote(field.Column)+" "+dialect.ContainsOperator()+" ?")
			args = append(args, "%"+value.(string)+"%")
			continue
		}
		parts = append(parts, quote(field.Column)+" "+op+" ?")
		args = append(args, normalize(dialect, field.Type, value))
	}
	return strings.Join(parts, " AND "), args, nil
}

func compatible(kind, op string, value any) bool {
	if value == nil {
		return op == "=" || op == "!="
	}
	switch kind {
	case "number":
		_, ok := value.(float64)
		return ok && op != "~"
	case "boolean":
		_, ok := value.(bool)
		return ok && (op == "=" || op == "!=")
	default:
		_, ok := value.(string)
		return ok && (op == "=" || op == "!=" || op == "~" || op == ">" || op == ">=" || op == "<" || op == "<=")
	}
}

func normalize(dialect store.Dialect, kind string, value any) any {
	if kind == "boolean" && value != nil {
		return dialect.Boolean(value.(bool))
	}
	return value
}

func quote(identifier string) string { return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"` }
