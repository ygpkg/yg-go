package types

import "time"

// Int is a helper routine that allocates a new int32 value
// to store v and returns a pointer to it, but unlike Int32
// its argument value is an int.
func Int(v int) *int {
	p := new(int)
	*p = v
	return p
}

func Int8(v int8) *int8 {
	p := new(int8)
	*p = v
	return p
}

func Int64(v int64) *int64 {
	p := new(int64)
	*p = v
	return p
}

// Uint is a helper routine that allocates a new uint32 value
// to store v and returns a pointer to it, but unlike Uint32
// its argument value is an int.
func Uint(v uint) *uint {
	p := new(uint)
	*p = v
	return p
}

func Uint8(v uint8) *uint8 {
	p := new(uint8)
	*p = v
	return p
}

func Float32(v float32) *float32 {
	p := new(float32)
	*p = v
	return p
}

func Float64(v float64) *float64 {
	p := new(float64)
	*p = v
	return p
}

// String is a helper routine that allocates a new string value
// to store v and returns a pointer to it.
func String(v string) *string {
	p := new(string)
	*p = v
	return p
}

// Time time.Time
func Time(t time.Time) *time.Time {
	p := new(time.Time)
	*p = t
	return p
}
