package usersync

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsPermanent(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"value too long 22001", &pgconn.PgError{Code: "22001"}, true},
		{"unique login 23505", &pgconn.PgError{Code: "23505"}, true},
		{"not null 23502", &pgconn.PgError{Code: "23502"}, true},
		{"invalid message sentinel", fmt.Errorf("wrap: %w", errInvalidMessage), true},
		{"wrapped pg error", fmt.Errorf("upsert: %w", &pgconn.PgError{Code: "22001"}), true},
		{"connection failure 08006 — transient", &pgconn.PgError{Code: "08006"}, false},
		{"plain error — transient", errors.New("dial tcp: connection refused"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPermanent(tc.err); got != tc.want {
				t.Fatalf("isPermanent(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
