package random

import "testing"

func TestNumber(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Logf("%02d: %v", i, Number(6))
	}
}

func TestNumber2(t *testing.T) {
	totalMap := map[rune]float64{}
	var total float64 = 0
	for i := 0; i < 20000; i++ {
		for _, v := range Number(6) {
			total++
			totalMap[v] += 1
		}
	}
	for k, v := range totalMap {
		t.Logf("%s: %.03f", string(k), v/total)
	}
}

func TestAlphabet(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Logf("%02d: %v", i, Alphabet(6))
	}
}

func TestAlphanum(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Logf("%02d: %v", i, Alphanum(6))
	}
}

func TestString(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Logf("%02d: %v", i, String(6))
	}
}
