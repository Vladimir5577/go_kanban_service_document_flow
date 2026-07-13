package helper

import "time"

// Clock centralizes the rules for storing and reading "wall clock" time
// in TIMESTAMP WITHOUT TIME ZONE columns.
//
// This logic lives in the helper package (per request to consolidate there).
//
// # Design Decision
//
// We store the *actual local (Moscow) wall time* directly in the database columns
// created_at, read_at, deleted_at, synced_at, updated_at etc.
//
//   - The columns are defined as TIMESTAMP(0) (no time zone).
//   - Postgres TIMESTAMP stores only the numeric components (year, month, day, hour...).
//   - By convention, those numbers always represent Europe/Moscow civil time.
//   - This satisfies the requirement that the actual date/time (фактическая дата) is stored and displayed.
//
// Why not TIMESTAMPTZ?
// The product requirement is to persist and return the real clock time that
// users and operators see in Moscow, rather than an absolute instant.
// Using wall time in TIMESTAMP makes the stored value directly usable
// for display and reporting without extra AT TIME ZONE conversions.
//
// # How it works
//
// Write path:
//   - Before inserting into the DB we convert the time so that its wall
//     components (Y/M/D h:m:s) correspond to Moscow time.
//   - time.Now() becomes Moscow wall time.
//   - Times coming from external systems (RabbitMQ) are converted via their
//     instant → Moscow wall time.
//
// Read path:
//   - pgx/pgtype.Timestamp for a TIMESTAMP column returns a time.Time whose
//     numeric components match what is stored in the DB, but usually with
//     location set to UTC (or time.Local).
//   - We must reinterpret those exact numbers with the Moscow location
//     attached, otherwise t.Format(...) and JSON output would be correct by
//     accident, but t.In(...) and other operations would give wrong results.
//
// # Connection to other parts
//
// - Container TZ=Europe/Moscow and DB connection parameter "timezone=Europe/Moscow"
//   ensure that SQL expressions like NOW() also produce Moscow wall time.
// - All Go code that creates times for persistence should go through these helpers.
//
// Do not scatter .In(loc) and time.Date(...) logic across the codebase.
type Clock struct {
	loc *time.Location
}

// NewClock returns a Clock bound to the provided location.
// If loc is nil, UTC is used as a safe fallback.
func NewClock(loc *time.Location) Clock {
	if loc == nil {
		loc = time.UTC
	}
	return Clock{loc: loc}
}

// Location returns the time.Location used by this Clock (normally Europe/Moscow).
func (c Clock) Location() *time.Location {
	if c.loc == nil {
		return time.UTC
	}
	return c.loc
}

// Now returns the current time expressed as wall time in the clock's location.
// Use this (or ToWall) when you need a timestamp to store in the database.
func (c Clock) Now() time.Time {
	return time.Now().In(c.Location())
}

// ToWall returns a time whose wall-clock fields (year, month, day, hour, ...)
// represent the clock's location.
//
// Typical usage: prepare a time before putting it into pgtype.Timestamp
// for a TIMESTAMP WITHOUT TIME ZONE column.
func (c Clock) ToWall(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.In(c.Location())
}

// ToWallPtr is the pointer version of ToWall. It returns nil if input is nil.
func (c Clock) ToWallPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	w := c.ToWall(*t)
	return &w
}

// FromDB takes a time value loaded from a TIMESTAMP (no TZ) column and returns
// a time.Time that has the *same numeric components* but with the clock's
// location attached.
//
// This is required because the database and pgx do not carry zone information
// for TIMESTAMP columns. The numbers we stored must be interpreted as Moscow
// wall time when we bring them back into Go.
//
// Use this in mappers that convert dbgen / pgtype values to domain models.
func (c Clock) FromDB(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	loc := c.Location()
	return time.Date(
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
		loc,
	)
}

// FromDBPtr is the pointer version of FromDB.
func (c Clock) FromDBPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	w := c.FromDB(*t)
	return &w
}

// IsZero reports whether the clock has no location configured (should not happen in practice).
func (c Clock) IsZero() bool {
	return c.loc == nil
}
