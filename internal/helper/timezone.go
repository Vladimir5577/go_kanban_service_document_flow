package helper

import "time"

// moscow is loaded once. Falls back to fixed +03:00 (Moscow has no DST since 2014).
var moscow = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return time.FixedZone("MSK", 3*60*60)
	}
	return loc
}()

// MoscowLocation returns Europe/Moscow location.
// Use ONLY for formatting human-readable strings (e.g. activity notifications).
// Never use for JSON API output or DB logic — those are always UTC with Z.
func MoscowLocation() *time.Location { return moscow }
