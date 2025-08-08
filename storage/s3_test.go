package storage

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/ygpkg/yg-go/config"
)

// TestNewS3Fs 连接S3存储 测试通过：minio cos
func TestNewS3Fs(t *testing.T) {
	var defaultCfg = config.S3StorageConfig{
		EndPoint:        "必须用http:// 用https会有问题",
		AccessKeyID:     "",
		SecretAccessKey: "",
		Bucket:          "",
		Region:          "ap-",
		UsePathStyle:    true, // 目前测试minio为true cos为false
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
	s3c, err := NewS3Fs(defaultCfg, config.StorageOption{})
	if err != nil {
		fmt.Println(err)
	}
	t.Logf("%+v", s3c)
}

// TestS3FsSave 保存文件 测试通过：minio cos
func TestS3FsSave(t *testing.T) {
	var defaultCfg = config.S3StorageConfig{
		EndPoint:        "必须用http:// 用https会有问题",
		AccessKeyID:     "",
		SecretAccessKey: "",
		Bucket:          "",
		Region:          "ap-",
		UsePathStyle:    true, // 目前测试minio为true cos为false
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
	s3c, err := NewS3Fs(defaultCfg, config.StorageOption{})
	if err != nil {
		fmt.Println(err)
	}
	content := []byte("this is a test")
	// 腾讯云会更改文件内容
	path := "test/a/test1.txt"
	if err := s3c.Save(context.Background(), &FileInfo{StoragePath: path, Size: int64(len(content))}, bytes.NewBuffer(content)); err != nil {
		fmt.Println(err)
	}
}

// TestS3FsGetPresignedURL 预上传预下载 测试通过：minio cos
func TestS3FsGetPresignedURL(t *testing.T) {
	var defaultCfg = config.S3StorageConfig{
		EndPoint:        "必须用http:// 用https会有问题",
		AccessKeyID:     "",
		SecretAccessKey: "",
		Bucket:          "",
		Region:          "ap-",
		UsePathStyle:    true, // 目前测试minio为true cos为false
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
	s3c, err := NewS3Fs(defaultCfg, config.StorageOption{})
	if err != nil {
		fmt.Println(err)
	}
	path := "test/a/test4.txt"
	url, err := s3c.GetPresignedURL(http.MethodPut, path)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(url)
}

// TestS3FsReadFile 读取文件 测试通过：minio cos
func TestS3FsReadFile(t *testing.T) {
	var defaultCfg = config.S3StorageConfig{
		EndPoint:        "必须用http:// 用https会有问题",
		AccessKeyID:     "",
		SecretAccessKey: "",
		Bucket:          "",
		Region:          "ap-",
		UsePathStyle:    true, // 目前测试minio为true cos为false
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
	s3c, err := NewS3Fs(defaultCfg, config.StorageOption{})
	if err != nil {
		fmt.Println(err)
	}
	path := "test/a/test2.txt"
	// file, err := s3c.ReadFile(path)
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// data, _ := io.ReadAll(file)
	// fmt.Println(string(data))
	fmt.Println(s3c.GetPublicURL(path, false))
}

// TestS3FsDeleteFile 删除 测试通过：minio cos
func TestS3FsDeleteFile(t *testing.T) {
	var defaultCfg = config.S3StorageConfig{
		EndPoint:        "必须用http:// 用https会有问题",
		AccessKeyID:     "",
		SecretAccessKey: "",
		Bucket:          "",
		Region:          "ap-",
		UsePathStyle:    true, // 目前测试minio为true cos为false
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
	s3c, err := NewS3Fs(defaultCfg, config.StorageOption{})
	if err != nil {
		fmt.Println(err)
	}
	path := "test/a/test2.txt"
	err = s3c.DeleteFile(path)
	if err != nil {
		fmt.Println(err)
	}
}

// TestS3FsCopyDir 复制 测试通过：minio cos
func TestS3FsCopyDir(t *testing.T) {
	var defaultCfg = config.S3StorageConfig{
		EndPoint:        "必须用http:// 用https会有问题",
		AccessKeyID:     "",
		SecretAccessKey: "",
		Bucket:          "",
		Region:          "ap-",
		UsePathStyle:    true, // 目前测试minio为true cos为false
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
	s3c, err := NewS3Fs(defaultCfg, config.StorageOption{})
	if err != nil {
		fmt.Println(err)
	}
	path := "test/a"
	err = s3c.CopyDir(path, "test/b")
	if err != nil {
		fmt.Println(err)
	}
}

// TestS3FsCopyDir 复制 测试通过：minio cos
func TestS3FsUploadDirectory(t *testing.T) {
	var defaultCfg = config.S3StorageConfig{
		EndPoint:        "",
		AccessKeyID:     "",
		SecretAccessKey: "",
		Bucket:          "",
		Region:          "ap-",
		UsePathStyle:    true, // 目前测试minio为true cos为false
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" {
		t.Skip("skip test, no minoss config")
		return
	}
	s3c, err := NewS3Fs(defaultCfg, config.StorageOption{})
	if err != nil {
		fmt.Println(err)
	}
	paths, err := s3c.UploadDirectory("/usr/local/goProject/src/yg-go/storage/weedfs", "test/b")
	fmt.Println(paths, err)
}
