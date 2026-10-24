package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type userStatusStub struct {
	active bool
	err    error
	calls  int
}

func (s *userStatusStub) IsUserActive(_ context.Context, id int64) (bool, error) {
	s.calls++
	return s.active && id == 7, s.err
}

func TestAuthRechecksUserOnEveryRequest(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"id": 7, "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	status := &userStatusStub{active: true}
	m := &AuthMiddleware{publicKey: &key.PublicKey, users: status}
	called := 0
	h := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		name   string
		active bool
		err    error
		want   int
	}{
		{"active", true, nil, 204},
		{"deleted or missing", false, nil, 401},
		{"lookup fails closed", true, errors.New("database unavailable"), 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status.active, status.err = tc.active, tc.err
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
	if called != 1 || status.calls != 3 {
		t.Fatalf("handler=%d lookups=%d", called, status.calls)
	}
}
