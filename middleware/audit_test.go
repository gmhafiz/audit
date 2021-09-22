package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gmhafiz/audit"
)

func TestAudit(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value("audit")
		if val == nil {
			t.Error("audit not present")
		}
		_, ok := val.(audit.Event)
		if !ok {
			t.Error("incorrect type")
		}
	})

	handlerToTest := Audit(nextHandler)

	req := httptest.NewRequest("GET", "http://localhost.test", nil)

	handlerToTest.ServeHTTP(httptest.NewRecorder(), req)
}
