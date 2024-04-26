package nettools

import (
	"net"
)

// CheckPort checks if the port is open
func CheckPort(addr string) bool {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
