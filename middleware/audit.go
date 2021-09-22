package middleware

import (
	"context"
	"net/http"

	"github.com/gmhafiz/audit"
)

type key string

const (
	UserID key = "userID"
)

func Audit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ev := audit.Event{
			ActorID:    getUserID(r),
			HTTPMethod: r.Method,
			URL:        readUserIP(r),
			IPAddress:  r.Host,
			UserAgent:  r.UserAgent(),
		}

		ctx := context.WithValue(r.Context(), "audit", ev)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getUserID(r *http.Request) uint64 {
	val, _ := r.Context().Value(UserID).(uint64)
	return val
}

func readUserIP(r *http.Request) string {
	ipAddress := r.Header.Get("X-Real-Ip")
	if ipAddress == "" {
		ipAddress = r.Header.Get("X-Forwarded-For")
	}
	if ipAddress == "" {
		ipAddress = r.RemoteAddr
	}
	return ipAddress
}
