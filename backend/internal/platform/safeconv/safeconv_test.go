package safeconv

import (
	"math"
	"testing"
)

func TestSignedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		in     int
		want32 int32
		want16 int16
	}{
		{math.MinInt, math.MinInt32, math.MinInt16},
		{math.MinInt32 - 1, math.MinInt32, math.MinInt16},
		{math.MinInt32, math.MinInt32, math.MinInt16},
		{math.MinInt16 - 1, math.MinInt16 - 1, math.MinInt16},
		{math.MinInt16, math.MinInt16, math.MinInt16},
		{-1, -1, -1},
		{0, 0, 0},
		{1, 1, 1},
		{math.MaxInt16, math.MaxInt16, math.MaxInt16},
		{math.MaxInt16 + 1, math.MaxInt16 + 1, math.MaxInt16},
		{math.MaxInt32, math.MaxInt32, math.MaxInt16},
		{math.MaxInt32 + 1, math.MaxInt32, math.MaxInt16},
		{math.MaxInt, math.MaxInt32, math.MaxInt16},
	} {
		if got := ToInt32(tc.in); got != tc.want32 {
			t.Errorf("ToInt32(%d) = %d, want %d", tc.in, got, tc.want32)
		}
		if got := ToInt16(tc.in); got != tc.want16 {
			t.Errorf("ToInt16(%d) = %d, want %d", tc.in, got, tc.want16)
		}
	}
}

func TestUnsignedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want uint32
	}{
		{math.MinInt64, 0},
		{-1, 0},
		{0, 0},
		{1, 1},
		{math.MaxUint32, math.MaxUint32},
		{math.MaxUint32 + 1, math.MaxUint32},
		{math.MaxInt64, math.MaxUint32},
	} {
		if got := ToUint32(tc.in); got != tc.want {
			t.Errorf("ToUint32(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
	for _, tc := range []struct {
		in   uint64
		want uint8
	}{
		{0, 0}, {1, 1}, {254, 254}, {255, 255}, {256, 255}, {math.MaxUint64, 255},
	} {
		if got := ToUint8(tc.in); got != tc.want {
			t.Errorf("ToUint8(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestUnsignedParserConversions(t *testing.T) {
	if ToInt(42) != 42 || ToInt(^uint64(0)) != math.MaxInt {
		t.Fatal("native int conversion wraps")
	}
	if ToInt64(42) != 42 || ToInt64(^uint64(0)) != math.MaxInt64 {
		t.Fatal("int64 conversion wraps")
	}
}

func TestUnsignedLengths(t *testing.T) {
	if ToUint64(-1) != 0 || ToUint64(42) != 42 || ToUint64(math.MaxInt) != uint64(math.MaxInt) {
		t.Fatal("length conversion wraps")
	}
}
