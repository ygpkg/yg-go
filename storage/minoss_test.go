package storage

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/ygpkg/yg-go/config"
)

// 连接Minoss
func TestNewMinBucketClient(t *testing.T) {
	var defaultCfg = config.MinossConfig{
		EndPoint:        os.Getenv("END_POINT"),
		AccessKeyID:     os.Getenv("ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("SECRET_ACCESS_KEY_ID"),
		Bucket:          "default-bucket",
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
	mc, err := NewMinFs(defaultCfg, config.StorageOption{})
	if err != nil {
		fmt.Println(err)
		t.Logf(err.Error())
		//t.Fail()

	}
	t.Logf("%+v", mc)
}

func TestMinBucketClient_UploadFile(t *testing.T) {
	var defaultCfg = config.MinossConfig{
		EndPoint:        os.Getenv("END_POINT"),
		AccessKeyID:     os.Getenv("ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("SECRET_ACCESS_KEY_ID"),
		Bucket:          "default-bucket",
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
	mc, err := NewMinFs(defaultCfg, config.StorageOption{})
	if err != nil {
		t.Logf(err.Error())
	}
	content := []byte("this is a test")
	path := "test/a/test.txt"
	if err := mc.Save(context.Background(), &FileInfo{StoragePath: path, Size: int64(len(content))}, bytes.NewBuffer(content)); err != nil {
		t.Logf(err.Error())
	}
}

func TestMinBucketClient_GetPresignedURL(t *testing.T) {
	var defaultCfg = config.MinossConfig{
		EndPoint:        os.Getenv("END_POINT"),
		AccessKeyID:     os.Getenv("ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("SECRET_ACCESS_KEY_ID"),
		Bucket:          "default-bucket",
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
	mc, err := NewMinFs(defaultCfg, config.StorageOption{})
	if err != nil {
		t.Logf(err.Error())
	}
	path := "test/a/test.txt"
	url, err := mc.GetPresignedURL(path)
	if err != nil {
		t.Logf(err.Error())
	}
	fmt.Println(url)
}

func TestMinBucketClient_ReadFile(t *testing.T) {
	var defaultCfg = config.MinossConfig{
		EndPoint:        os.Getenv("END_POINT"),
		AccessKeyID:     os.Getenv("ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("SECRET_ACCESS_KEY_ID"),
		Bucket:          "default-bucket",
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
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
	var defaultCfg = config.MinossConfig{
		EndPoint:        os.Getenv("END_POINT"),
		AccessKeyID:     os.Getenv("ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("SECRET_ACCESS_KEY_ID"),
		Bucket:          "default-bucket",
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
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
