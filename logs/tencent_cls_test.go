package logs

import (
	"os"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/config"
)

func TestTencentClsSyncer(t *testing.T) {
	lw := newLoggerWrapper("default", "main", []config.LogConfig{{
		Writer: "tencentcls",
		Level:  -1,
		TencentCLS: &config.TencentCLSConfig{
			TencentConfig: config.TencentConfig{
				Endpoint:  "ap-beijing.cls.tencentcs.com",
				Region:    "ap-beijing",
				SecretID:  os.Getenv("TENCENT_SECRET_ID"),
				SecretKey: os.Getenv("TENCENT_SECRET_KEY"),
			},
			TopicID: os.Getenv("TENCENT_TOPIC_ID"),
		},
	}, {
		Writer: "console",
		Level:  -1,
	}})

	for i := 0; i < 100; i++ {
		lw.logger.Infof("hahah %s, %v", "asdfasdafasdf", i)
	}
	Close()
	time.Sleep(time.Second * 2)
	t.Error("args ...any")
}
