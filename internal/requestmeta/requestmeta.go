package requestmeta

import (
	"context"
	"net"
	"net/http"
)

type key int

const (
	schemeKey key = iota
	clientIPKey
)

func With(r *http.Request, scheme, clientIP string) *http.Request {
	ctx := context.WithValue(r.Context(), schemeKey, scheme)
	ctx = context.WithValue(ctx, clientIPKey, clientIP)
	return r.WithContext(ctx)
}

func Scheme(r *http.Request) string {
	if value, ok := r.Context().Value(schemeKey).(string); ok && value != "" {
		return value
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func ClientIP(r *http.Request) string {
	if value, ok := r.Context().Value(clientIPKey).(string); ok && value != "" {
		return value
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
