package logs

// import (
// 	"testing"

// 	"github.com/ygpkg/yg-go/config"
// )

// func TestNewAliyunSlsSyncer(t *testing.T) {
// 	lw := newLoggerWrapper("default", "main", []config.LogConfig{{
// 		Writer: "aliyunsls",
// 		Level:  -1,
// 		AliyunSLS: &config.AliyunSLSConfig{
// 			AliConfig: config.AliConfig{
// 				Endpoint:        "cn-zhangjiakou.log.aliyuncs.com",
// 				RegionID:        "cn-zhangjiakou",
// 				AccessKeyID:     "",
// 				AccessKeySecret: "",
// 			},
// 			Project:  "roc-prod",
// 			Logstore: "ls-roc-prod",
// 		},
// 	}, {
// 		Writer: "console",
// 		Level:  -1,
// 	}})

// 	for i := 0; i < 100; i++ {
// 		lw.logger.Infof("hahah %s, %v", "asdfasdafasdf", i)
// 	}
// 	t.Error("args ...any")
// }
