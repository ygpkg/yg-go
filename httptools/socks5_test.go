package httptools

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ygpkg/yg-go/config"
)

func TestProxyClient(t *testing.T) {
	proxyAddr := os.Getenv("TEST_PROXY_ADDR")
	if proxyAddr == "" {
		t.Skip("no proxy addr")
		return
	}

	opt := config.ProxyConfig{
		Scheme: "socks5",
		Addr:   proxyAddr,
	}
	cli := ProxyClient(opt)
	if cli == nil {
		t.Error("ProxyClient() returned nil")
		return
	}

	res, err := cli.Get("http://myip.ipip.net/")
	if err != nil {
		t.Error(err)
		return
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
		return
	}

	t.Log(string(body))
}
