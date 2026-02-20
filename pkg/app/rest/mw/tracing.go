package mw

import (
	"context"
	"math/rand"
	"net/http"

	"github.com/Educentr/go-project-starter-runtime/pkg/logger"
	"github.com/Educentr/go-project-starter-runtime/pkg/reqctx"
)

const (
	// RequestIDHeader is a header name for request ID
	RequestIDHeader = "x-req-id"
	LengthRequestID = 10
)

var alphabet = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func randStr(size int) string {
	b := make([]rune, size)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}

	b[0] = 'r'
	b[1] = 'n'
	b[2] = 'd'

	return string(b)
}

func getTracingID(r *http.Request) string {
	requestID := r.Header.Get(RequestIDHeader)
	if requestID == "" {
		requestID = randStr(LengthRequestID)
	}

	return requestID
}

func HTTPServerMiddlewareTracing(_ context.Context) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			xReqID := getTracingID(r)

			ctx, err := reqctx.SetRequestID(r.Context(), xReqID)
			if err != nil {
				logger.GetEventLogger().Error(ctx, "Error set RequestID to context", err)
			} else {
				r = r.WithContext(ctx)
			}

			w.Header().Set("X-Req-ID", xReqID)

			next.ServeHTTP(w, r)
		})
	}
}
