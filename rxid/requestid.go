package rxid

import (
	"context"
	"net/http"

	"github.com/rs/xid"
)

type key int

const (
	requestIDKey key = 0
)

// NewContextWithID will get the incoming context off of the request or create
// a new one. The ctx with the ID is returned.
func NewContextWithID(ctx context.Context, r *http.Request) context.Context {
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = xid.New().String()
	}

	return context.WithValue(ctx, requestIDKey, reqID)
}

// FromContext returns the RequestID value. It panics if not set.
func FromContext(ctx context.Context) string {
	return ctx.Value(requestIDKey).(string)
}

// Handler wraps other handlers and adds the request id to incoming requests as
// well as sets the request id on the response.
func Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := NewContextWithID(r.Context(), r)
		next.ServeHTTP(w, r.WithContext(ctx))
		w.Header().Set("X-Request-ID", FromContext(ctx))
	})
}
