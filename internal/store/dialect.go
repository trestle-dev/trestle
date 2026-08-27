package store

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func QuoteIdentifier(value string) (string, error) {
	if !identifierPattern.MatchString(value) {
		return "", errors.New("invalid internal SQL identifier")
	}
	return `"` + value + `"`, nil
}

// RebindPostgres rewrites SQLite-style placeholders while preserving question
// marks inside SQL strings, quoted identifiers and comments.
func RebindPostgres(query string) (string, error) {
	var out strings.Builder
	out.Grow(len(query) + 8)
	state := byte(0)
	parameter := 0
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch state {
		case '\'':
			out.WriteByte(c)
			if c == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					i++
					out.WriteByte(query[i])
				} else {
					state = 0
				}
			}
		case '"':
			out.WriteByte(c)
			if c == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					i++
					out.WriteByte(query[i])
				} else {
					state = 0
				}
			}
		case '-':
			out.WriteByte(c)
			if c == '\n' {
				state = 0
			}
		case '/':
			out.WriteByte(c)
			if c == '*' && i+1 < len(query) && query[i+1] == '/' {
				i++
				out.WriteByte('/')
				state = 0
			}
		default:
			switch {
			case c == '\'' || c == '"':
				state = c
				out.WriteByte(c)
			case c == '-' && i+1 < len(query) && query[i+1] == '-':
				state = '-'
				out.WriteString("--")
				i++
			case c == '/' && i+1 < len(query) && query[i+1] == '*':
				state = '/'
				out.WriteString("/*")
				i++
			case c == '?':
				parameter++
				out.WriteByte('$')
				out.WriteString(strconv.Itoa(parameter))
			case c == '$' && i+1 < len(query) && query[i+1] >= '0' && query[i+1] <= '9':
				return "", errors.New("mixed SQL placeholder styles")
			default:
				out.WriteByte(c)
			}
		}
	}
	if state == '\'' || state == '"' || state == '/' {
		return "", errors.New("unterminated SQL quote or comment")
	}
	return out.String(), nil
}

type ErrorKind string

const (
	ErrorUnknown       ErrorKind = "unknown"
	ErrorUnique        ErrorKind = "unique"
	ErrorForeignKey    ErrorKind = "foreign_key"
	ErrorCheck         ErrorKind = "check"
	ErrorSerialization ErrorKind = "serialization"
)

func ClassifyError(err error) ErrorKind {
	if err == nil {
		return ErrorUnknown
	}
	var pg *pq.Error
	if errors.As(err, &pg) {
		switch string(pg.Code) {
		case "23505":
			return ErrorUnique
		case "23503":
			return ErrorForeignKey
		case "23514":
			return ErrorCheck
		case "40001", "40P01":
			return ErrorSerialization
		}
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed: unique"):
		return ErrorUnique
	case strings.Contains(message, "foreign key constraint"):
		return ErrorForeignKey
	case strings.Contains(message, "check constraint"):
		return ErrorCheck
	default:
		return ErrorUnknown
	}
}

func Bind(d Dialect, query string) string {
	bound, err := d.Bind(query)
	if err != nil {
		panic(fmt.Sprintf("invalid internal SQL: %v", err))
	}
	return bound
}
