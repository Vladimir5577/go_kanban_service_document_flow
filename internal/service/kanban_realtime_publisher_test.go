package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestKanbanRealtimePublisherPublishesMercureUpdate(t *testing.T) {
	const secret = "test-secret"
	called := false

	publisher := NewKanbanRealtimePublisher("http://mercure/.well-known/mercure", secret, nil, nil, nil, nil, nil, nil, nil)
	publisher.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true

		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("content type = %s, want application/x-www-form-urlencoded", got)
		}

		tokenString := r.Header.Get("Authorization")
		if len(tokenString) <= len("Bearer ") || tokenString[:len("Bearer ")] != "Bearer " {
			t.Fatalf("missing bearer token")
		}

		token, err := jwt.Parse(tokenString[len("Bearer "):], func(token *jwt.Token) (any, error) {
			return []byte(secret), nil
		})
		if err != nil {
			t.Fatalf("parse token: %v", err)
		}
		if !token.Valid {
			t.Fatalf("token is invalid")
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			t.Fatalf("claims type = %T, want jwt.MapClaims", token.Claims)
		}
		mercure, ok := claims["mercure"].(map[string]any)
		if !ok {
			t.Fatalf("mercure claim = %#v", claims["mercure"])
		}
		publish, ok := mercure["publish"].([]any)
		if !ok || len(publish) != 1 || publish[0] != "*" {
			t.Fatalf("mercure.publish = %#v, want [*]", mercure["publish"])
		}

		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("topic"); got != "/kanban/board/42" {
			t.Fatalf("topic = %s, want /kanban/board/42", got)
		}

		var event map[string]any
		if err := json.Unmarshal([]byte(r.Form.Get("data")), &event); err != nil {
			t.Fatalf("decode data: %v", err)
		}
		if event["type"] != "card_updated" {
			t.Fatalf("type = %v, want card_updated", event["type"])
		}
		if event["senderId"] != float64(7) {
			t.Fatalf("senderId = %v, want 7", event["senderId"])
		}

		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})}

	err := publisher.PublishCardUpdated(context.Background(), 42, map[string]any{
		"id":    int64(10),
		"title": "Card",
	}, 7)
	if err != nil {
		t.Fatalf("publish card updated: %v", err)
	}
	if !called {
		t.Fatalf("mercure endpoint was not called")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
