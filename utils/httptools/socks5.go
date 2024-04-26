package httptools

import (
	"fmt"
	"net/http"
	"os"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/settings"
	"github.com/ygpkg/yg-go/utils/logs"
	"golang.org/x/net/proxy"
)

func ProxyClient(opt config.ProxyConfig) *http.Client {
	if opt.Scheme == "socks5" {
		return ProxyClientSocks5(opt)
	}
	panic(fmt.Errorf("unsupported proxy protocol: %s", opt.Scheme))
	return nil
}

func ProxyClientSocks5(opt config.ProxyConfig) *http.Client {
	auth := &proxy.Auth{
		User:     opt.Username,
		Password: opt.Password,
	}

	// create a socks5 dialer
	dialer, err := proxy.SOCKS5("tcp", opt.Addr, auth, proxy.Direct)
	if err != nil {
		fmt.Fprintln(os.Stderr, "can't connect to the proxy:", err)
		os.Exit(1)
	}

	cli := &http.Client{
		// setup a http client
		Transport: &http.Transport{
			// set our socks5 as the dialer
			Dial: dialer.Dial,
		},
	}

	return cli
}

// ProxySocks5 set proxy for http client
func ProxySocks5FromSetting(cli *http.Client, group, key string) error {
	opt, err := ProxyConfigFromSetting(group, key)
	if err != nil {
		logs.Errorf("ProxySocks5FromSetting: get proxy config failed, %s", err)
		return err
	}
	auth := &proxy.Auth{
		User:     opt.Username,
		Password: opt.Password,
	}

	// create a socks5 dialer
	dialer, err := proxy.SOCKS5("tcp", opt.Addr, auth, proxy.Direct)
	if err != nil {
		logs.Errorf("ProxySocks5FromSetting: can't connect to the proxy: %s", err)
		return err
	}

	if cli.Transport == nil {
		cli.Transport = &http.Transport{
			Dial: dialer.Dial,
		}
	} else {
		if t, ok := cli.Transport.(*http.Transport); ok {
			t.Dial = dialer.Dial
		} else {
			logs.Errorf("ProxySocks5FromSetting: unsupported transport: %T", cli.Transport)
			return fmt.Errorf("unsupported transport: %T", cli.Transport)
		}
	}

	return nil
}

// ProxyConfigFromSetting get proxy config from settings
func ProxyConfigFromSetting(group, key string) (*config.ProxyConfig, error) {
	opt := &config.ProxyConfig{}
	err := settings.GetYaml(group, key, opt)
	if err != nil {
		logs.Errorf("ProxyConfigFromSetting: get proxy config failed, %s", err)
		return nil, err
	}
	return opt, nil
}
