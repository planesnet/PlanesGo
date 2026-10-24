package servidor

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A panic in a request answers 500 (and is reported) instead of cutting the connection; ErrAbortHandler is not a
// failure and goes on.
func TestRecoversPanics(t *testing.T) {
	s := &Servidor{}
	h := s.recovers(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		var m map[string]int
		m["x"] = 1
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/x", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("code %d", rec.Code)
	}
	defer func() {
		if recover() != http.ErrAbortHandler {
			t.Error("ErrAbortHandler must go on")
		}
	}()
	s.recovers(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}
