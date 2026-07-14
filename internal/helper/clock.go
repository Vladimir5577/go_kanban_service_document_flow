package helper

import "time"

// Clock даёт текущий момент (для тестируемости/мокабельности).
// Всё время в приложении — UTC, усечённое до секунды (колонки TIMESTAMPTZ(0)).
type Clock struct{}

func NewClock() Clock { return Clock{} }

// Now возвращает текущий момент в UTC, усечённый до секунды.
func (c Clock) Now() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

