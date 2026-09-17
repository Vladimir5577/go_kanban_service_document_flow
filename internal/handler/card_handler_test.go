package handler

import (
	"math"
	"testing"
)

func TestPageOffset(t *testing.T) {
	cases := []struct {
		name     string
		page     int
		pageSize int
		want     int32
	}{
		{"первая страница", 1, 20, 0},
		{"вторая страница", 2, 20, 20},
		{"обычная глубина", 5, 100, 400},
		// Ровно тот вход, на котором раньше случалось переполнение:
		// 29999999*100 = 2999999900 > math.MaxInt32, после int32 уходило в минус,
		// и база отвечала ошибкой на отрицательный OFFSET.
		{"переполнение int32", 30000000, 100, math.MaxInt32},
		{"без нижнего выхода за ноль", 0, 20, 0},
	}
	for _, c := range cases {
		got := pageOffset(c.page, c.pageSize)
		if got != c.want {
			t.Errorf("%s: pageOffset(%d, %d) = %d, want %d", c.name, c.page, c.pageSize, got, c.want)
		}
		if got < 0 {
			t.Errorf("%s: отрицательное смещение %d уедет в OFFSET", c.name, got)
		}
	}
}
