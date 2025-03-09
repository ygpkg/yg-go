package lifecycle

import (
	"fmt"
	"os"

	"github.com/ygpkg/yg-go/nettools"
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
	ip := nettools.MustLocalPrimearyIP()
	return fmt.Sprintf("%s-%s-%d", hostname, ip, pid)
}
