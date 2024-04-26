package nettools

import (
	"encoding/binary"
	"errors"
	"net"
)

// MustLocalPrimearyIP .
func MustLocalPrimearyIP() net.IP {
	inters, err := net.InterfaceAddrs()
	if err != nil {
		return net.ParseIP("127.0.0.1")
	}

	for i, _ := range inters {
		intr, err := net.InterfaceByIndex(i)
		if err != nil {
			continue
		}

		addrs, _ := intr.Addrs()
		for _, addr := range addrs {
			if ip, ok := addr.(*net.IPNet); ok && ip.IP.IsGlobalUnicast() {
				return ip.IP
			}
		}
	}
	return net.ParseIP("127.0.0.1")
}

// IPToUint32 将IPv4地址转换为uint32整数
func IPToUint32(ip string) (uint32, error) {
	addr := net.ParseIP(ip)
	if addr == nil {
		return 0, errors.New("invalid IP address")
	}
	if addr.To4() == nil {
		return 0, errors.New("not an IPv4 address")
	}
	return binary.BigEndian.Uint32(addr.To4()), nil
}

// MustIPToUint32 将IPv4地址转换为uint32整数
func MustIPToUint32(ip string) uint32 {
	n, _ := IPToUint32(ip)
	return n
}

// Uint32ToIP 将uint32整数转换为IPv4地址
func Uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}
