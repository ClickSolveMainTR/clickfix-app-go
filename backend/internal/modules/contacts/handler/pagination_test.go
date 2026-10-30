package handler

import "testing"

func TestPaginationIntegerBounds(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int32
	}{
		{"2147483647", 2147483647},
		{"-2147483648", -2147483648},
		{"2147483648", 0},
		{"4294967297", 0},
		{"-2147483649", 0},
		{"9223372036854775808", 0},
		{" 50 ", 50},
		{"invalid", 0},
	} {
		if got := atoi(tc.raw); got != tc.want {
			t.Errorf("atoi(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}
