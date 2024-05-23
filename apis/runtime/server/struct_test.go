package server

import "testing"

type TTUser struct {
	Name string
	Age  uint32

	ImageURL *string

	DDD []int

	TTFa TTFa `json:"tt"`

	EEE []*TTFa

	FFF map[string]int
	GGG map[uint32]*TTFa
}

type TTFa struct {
	AA uint64

	Next *TTFa
}

func TestParseStruct(t *testing.T) {
	parseInterface(&TTUser{})
}
