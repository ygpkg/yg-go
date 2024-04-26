package nettools

import "testing"

func TestIPv4ToUint32(t *testing.T) {
	for _, ip := range []string{
		"192.168.1.10",
		"8.8.8.8",
		"225.225.225.225",
		"0.0.0.0",
	} {
		ipuint32, err := IPToUint32(ip)
		if err != nil {
			t.Errorf("IPv4ToUint32(%s) = %v, want %v", ip, err, nil)
		}
		reP := Uint32ToIP(ipuint32)
		if reP.String() != ip {
			t.Errorf("IPv4ToUint32(%s) = %v, want %v", ip, reP, ip)
		}
		t.Logf("%s -> %v -> %s", ip, ipuint32, reP)
	}
}
