package storage

import (
	"fmt"
	"io/ioutil"
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

// func TestMinBucketClient_UploadFile(t *testing.T) {

// 	mc, err := NewMinFs(defaultCfg, config.StorageOption{})
// 	if err != nil {
// 		t.Logf(err.Error())
// 	}
// 	content := []byte("this is a test")
// 	path := "test/a/test.txt"
// 	if err := mc.Save(context.Background(), &FileInfo{StoragePath: path, Size: int64(len(content))}, bytes.NewBuffer(content)); err != nil {
// 		t.Logf(err.Error())
// 	}
// }

// func TestMinBucketClient_GetPresignedURL(t *testing.T) {

// 	mc, err := NewMinFs(defaultCfg, config.StorageOption{})
// 	if err != nil {
// 		t.Logf(err.Error())
// 	}
// 	path := "test/a/test.txt"
// 	url, err := mc.GetPresignedURL(path)
// 	if err != nil {
// 		t.Logf(err.Error())
// 	}
// 	fmt.Println(url)
// }

func TestMinBucketClient_ReadFile(t *testing.T) {

	mc, err := NewMinFs(defaultCfg, config.StorageOption{})
	if err != nil {
		t.Logf(err.Error())
	}
	path := "test/a/test.txt"
	file, err := mc.ReadFile(path)
	if err != nil {
		t.Logf(err.Error())
	}
	data, _ := ioutil.ReadAll(file)
	fmt.Println(string(data))
}

func TestMinBucketClient_DeleteFile(t *testing.T) {

	mc, err := NewMinFs(defaultCfg, config.StorageOption{})
	if err != nil {
		t.Logf(err.Error())
	}
	path := "test/a/test.txt"
	err = mc.DeleteFile(path)
	if err != nil {
		t.Logf(err.Error())
	}
}
