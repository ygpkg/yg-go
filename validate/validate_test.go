package validate

import "testing"

// TestPhone ...
func TestPhone(t *testing.T) {
	table := map[string]bool{
		"18002271357":  true,
		"15191481233":  true,
		"17302615398":  true,
		"19952343129":  true,
		"1800227135":   false,
		"1519148123":   false,
		"180022713522": false,
		"151914812322": false,
		"280022713522": false,
		"051914812322": false,
	}
	for ph, b := range table {
		if (IsPhone(ph) == nil) != b {
			t.Errorf("phone check failed, %v should %v", ph, b)
		}
	}
}

// TestIDCardNumber ...
func TestIDCardNumber(t *testing.T) {
	table := map[string]bool{
		"420521189212245026":  true,
		"42052118921224502":   false,
		"4205211892122450251": false,
		"42052118921224502X":  true,
		"42052118921224502x":  true,
	}
	for num, isErr := range table {
		if (IsCardNumber(num) == nil) != isErr {
			t.Errorf("idcard check failed, %v should %v", num, isErr)
		}
	}
}

// TestBankAccountNumber ...
func TestBankAccountNumber(t *testing.T) {
	table := map[string]bool{
		"4205211892122450":    true,
		"42052118921224502":   false,
		"4205211892122450251": true,
		"42052118921224502X":  false,
		"42052118921224502x":  false,
	}
	for num, isErr := range table {
		if (IsBankAccountNumber(num) == nil) != isErr {
			t.Errorf("idcard check failed, %v should %v", num, isErr)
		}
	}
}

func TestLetterNumber(t *testing.T) {
	table := map[string]bool{
		"4205211892122450":    true,
		"42052118921224502":   true,
		"4205211892122450251": true,
		"42052118921224502X":  true,
		"42052118921224502x":  true,
		"0":                   true,
		"":                    false,
		"a.d":                 false,
	}
	for num, isErr := range table {
		if (IsLetterNumber(num) == nil) != isErr {
			t.Errorf("LetterNumber check failed, %v should %v", num, isErr)
		}
	}
}
