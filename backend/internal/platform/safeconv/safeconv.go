// Package safeconv provides saturating integer conversions. Values outside the
// destination range are clamped to its nearest endpoint; they never wrap.
// Callers must validate domain constraints before storing identities or enums.
package safeconv

import "math"

// ToInt32 clamps v to the signed 32-bit range.
func ToInt32(v int) int32 {
	if v < math.MinInt32 {
		return math.MinInt32
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(v)
}

// ToInt16 clamps v to the signed 16-bit range.
func ToInt16(v int) int16 {
	if v < math.MinInt16 {
		return math.MinInt16
	}
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	return int16(v)
}

// ToUint32 clamps v to [0, 2^32-1].
func ToUint32(v int64) uint32 {
	if v < 0 {
		return 0
	}
	if v > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(v)
}

// ToUint8 clamps v to [0, 255].
func ToUint8(v uint64) uint8 {
	if v > math.MaxUint8 {
		return math.MaxUint8
	}
	return uint8(v)
}

// ToInt clamps an unsigned parser value to the native signed integer range.
func ToInt(v uint64) int {
	if v > math.MaxInt {
		return math.MaxInt
	}
	return int(v)
}

// ToInt64 clamps an unsigned parser value to the signed 64-bit range.
func ToInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// ToUint64 clamps a native signed length to the unsigned 64-bit range.
func ToUint64(v int) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}
