package dex

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPasswordLoginBurstRefillAndOtherRoutes(t *testing.T) {
	var calls atomic.Int32
	handler := limitLocalPasswordLogin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	request := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader("private-password-sentinel")))
		return rec
	}
	for attempt := 0; attempt < 20; attempt++ {
		require.Equal(t, http.StatusNoContent, request(http.MethodPost, "/oauth2/auth/local/login").Code)
	}
	rec := request(http.MethodPost, "/oauth2/auth/local/login?req=private-code-sentinel")
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "1", rec.Header().Get("Retry-After"))
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	require.Contains(t, rec.Header().Get("Content-Security-Policy"), "default-src 'none'")
	require.Contains(t, rec.Body.String(), "Aheadscale")
	require.NotContains(t, rec.Body.String(), "NetBird")
	require.NotContains(t, rec.Body.String(), "private-password-sentinel")
	require.NotContains(t, rec.Body.String(), "private-code-sentinel")
	require.Equal(t, int32(20), calls.Load())

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/oauth2/auth/local/login"},
		{http.MethodPost, "/oauth2/token"},
		{http.MethodPost, "/oauth2/signup"},
		{http.MethodGet, "/oauth2/callback"},
		{http.MethodPost, "/oauth2/callback"},
		{http.MethodGet, "/oauth2/auth/yandex"},
		{http.MethodPost, "/oauth2/auth/yandex/login"},
		{http.MethodGet, "/oauth2/logout"},
	} {
		require.Equal(t, http.StatusNoContent, request(route.method, route.path).Code, route.path)
	}

	<-time.After(1100 * time.Millisecond)
	require.Equal(t, http.StatusNoContent, request(http.MethodPost, "/oauth2/auth/local/login").Code)
}

func TestPasswordLoginConcurrencyAndRelease(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var once sync.Once
	var pending sync.WaitGroup
	handler := limitLocalPasswordLogin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		pending.Wait()
	})
	for i := 0; i < 2; i++ {
		pending.Add(1)
		go func() {
			defer pending.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/oauth2/auth/local/login", nil))
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("password request did not reach the handler")
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/oauth2/auth/local/login", nil))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "1", rec.Header().Get("Retry-After"))
	once.Do(func() { close(release) })
	pending.Wait()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/oauth2/auth/local/login", nil))
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPasswordLoginSlotsReleasedAfterPanic(t *testing.T) {
	var calls int
	handler := limitLocalPasswordLogin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls <= 2 {
			panic("synthetic downstream failure")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for i := 0; i < 2; i++ {
		require.Panics(t, func() {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/oauth2/auth/local/login", nil))
		})
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/oauth2/auth/local/login", nil))
	require.Equal(t, http.StatusNoContent, rec.Code)
}
