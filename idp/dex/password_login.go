package dex

import (
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// limitLocalPasswordLogin bounds password verification work across all local
// accounts. Other OAuth endpoints must remain usable when this bucket is empty.
func limitLocalPasswordLogin(next http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Every(time.Second), 20)
	working := make(chan struct{}, 2)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/auth/"+LocalConnectorID+"/login") {
			next.ServeHTTP(w, r)
			return
		}
		if !limiter.Allow() {
			passwordLoginRetry(w)
			return
		}
		select {
		case working <- struct{}{}:
			defer func() { <-working }()
		default:
			passwordLoginRetry(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func passwordLoginRetry(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "1")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = io.WriteString(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Sign in — Aheadscale</title><style>:root{color-scheme:light dark}body{font:16px/1.5 system-ui,sans-serif;margin:0;padding:48px 20px;background:#f5f7fa;color:#172033}main{max-width:440px;margin:10vh auto;padding:32px;border:1px solid #cbd5e1;border-radius:16px}strong{color:#2563eb}h1{font-size:24px}@media(prefers-color-scheme:dark){body{background:#171b1e;color:#e2e5e7}main{border-color:#47515c}}</style></head><body><main><strong>Aheadscale</strong><h1>Please wait before signing in</h1><p>Too many sign-in attempts. Wait a moment, then go back and try again.</p><p>If you closed this tab, return to Aheadscale and start a new sign-in attempt.</p></main></body></html>`)
}
