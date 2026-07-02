package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireWriter(t *testing.T) {
	cases := []struct {
		name       string
		session    *SessionInfo
		wantStatus int
	}{
		{"admin allowed", &SessionInfo{UserRole: "admin"}, http.StatusOK},
		{"analyst allowed", &SessionInfo{UserRole: "analyst"}, http.StatusOK},
		{"viewer blocked", &SessionInfo{UserRole: "viewer"}, http.StatusForbidden},
		{"empty role blocked", &SessionInfo{UserRole: ""}, http.StatusForbidden},
		{"no session blocked", nil, http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := RequireWriter()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodPost, "/dashboards", nil)
			if tc.session != nil {
				req = req.WithContext(SetSession(req.Context(), tc.session))
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s: got %d, want %d", tc.name, rec.Code, tc.wantStatus)
			}
		})
	}
}
