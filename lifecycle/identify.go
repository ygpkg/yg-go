package lifecycle

import (
	"fmt"
	"os"
)

// OwnerID 获取当前服务的ownerID
func OwnerID() string {
	hostname := ""
	name, err := os.Hostname()
	if err == nil {
		hostname = name
	} else {
		hostname = "unknown"
	}
	pid := os.Getpid()
	return fmt.Sprintf("%s-%d", hostname, pid)
}
