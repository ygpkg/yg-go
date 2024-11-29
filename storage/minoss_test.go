package storage

import (
	"fmt"
	"testing"

	"github.com/ygpkg/yg-go/config"
)

var defaultCfg = config.MinossConfig{
	EndPoint:        "out.yygu.cn:53014",
	AccessKeyID:     "ygadmin",
	SecretAccessKey: "tqh9kby01lwqe8ec",
	Bucket:          "default-bucket",
}

// 连接Minoss
func TestNewMinBucketClient(t *testing.T) {

	mc, err := NewMinFs(defaultCfg, config.StorageOption{})
	if err != nil {
		fmt.Println(err)
		t.Logf(err.Error())
		//t.Fail()

	}
	t.Logf("%+v", mc)
}
