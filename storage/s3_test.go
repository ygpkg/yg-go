package storage

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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

// TestS3FsMultipartUpload
// ❯ go test -timeout 3m -run ^TestS3FsMultipartUpload_7MB$ github.com/ygpkg/yg-go/storage -v
func TestS3FsMultipartUpload_7MB(t *testing.T) {
	var defaultCfg = config.S3StorageConfig{
		UsePathStyle: true,
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" || defaultCfg.Bucket == "" {
		t.Skip("skip test, no S3/minio config")
		return
	}

	s3c, err := NewS3Fs(defaultCfg, config.StorageOption{})
	if err != nil {
		t.Fatalf("NewS3Fs error: %v", err)
	}

	ctx := context.Background()
	storagePath := "test/multipart/test-7mb.bin"

	// 初始化上传
	var uploadID *string
	t.Run("init", func(t *testing.T) {
		in := &CreateMultipartUploadInput{StoragePath: &storagePath}
		upID, err := s3c.CreateMultipartUpload(ctx, in)
		if err != nil {
			t.Fatalf("CreateMultipartUpload error: %v", err)
		}
		uploadID = upID
	})

	// 生成10MB内容
	const fileSize = 7 * 1024 * 1024 // 7 MB
	content := make([]byte, fileSize)
	for i := range content {
		content[i] = byte('a' + i%26) // 填充可打印字符，便于调试
	}

	// 分片上传
	const partSize = 5 * 1024 * 1024 // 5 MB
	partETags := make(map[int]string)
	partCount := (fileSize + partSize - 1) / partSize // 向上取整

	for i := 0; i < partCount; i++ {
		i := i
		t.Run(fmt.Sprintf("upload-part-%d", i+1), func(t *testing.T) {
			if uploadID == nil || *uploadID == "" {
				t.Fatalf("uploadID empty")
			}
			start := i * partSize
			end := (i + 1) * partSize
			if end > len(content) {
				end = len(content)
			}
			pn := i + 1
			in := &UploadPartInput{
				StoragePath: &storagePath,
				UploadID:    uploadID,
				PartNumber:  &pn,
				Data:        bytes.NewReader(content[start:end]),
			}
			etag, err := s3c.UploadPart(ctx, in)
			if err != nil {
				t.Fatalf("UploadPart %d error: %v", pn, err)
			}
			if etag == nil || *etag == "" {
				t.Fatalf("UploadPart %d returned empty ETag", pn)
			}
			partETags[pn] = *etag
		})
	}

	// 完成上传
	t.Run("complete", func(t *testing.T) {
		if uploadID == nil || *uploadID == "" {
			t.Fatalf("uploadID empty before complete")
		}
		parts := make([]types.CompletedPart, 0, len(partETags))
		for i := 1; i <= len(partETags); i++ { // S3要求按PartNumber顺序
			etag := partETags[i]
			pn := int32(i)
			parts = append(parts, types.CompletedPart{PartNumber: &pn, ETag: &etag})
		}
		cmp := &types.CompletedMultipartUpload{Parts: parts}
		in := &CompleteMultipartUploadInput{
			StoragePath: &storagePath,
			UploadID:    uploadID,
			Parts:       cmp,
		}
		if err := s3c.CompleteMultipartUpload(ctx, in); err != nil {
			t.Fatalf("CompleteMultipartUpload error: %v", err)
		}
	})
}

// TestS3FsPresignedUpload
func TestS3FsPresignedUpload(t *testing.T) {
	// Runs only with valid S3/MinIO config
	var defaultCfg = config.S3StorageConfig{
		UsePathStyle: true,
	}
	if defaultCfg.EndPoint == "" || defaultCfg.AccessKeyID == "" || defaultCfg.SecretAccessKey == "" || defaultCfg.Bucket == "" {
		t.Skip("skip test, no S3/minio config")
		return
	}

	s3c, err := NewS3Fs(defaultCfg, config.StorageOption{})
	if err != nil {
		t.Fatalf("NewS3Fs error: %v", err)
	}

	ctx := context.Background()
	method := http.MethodPut
	path := "test/presigned/test-presigned.bin"

	t.Run("direct-put-presigned", func(t *testing.T) {
		url, err := s3c.GeneratePresignedURL(ctx, &GeneratePresignedURLInput{
			Method:      &method,
			StoragePath: &path,
		})
		if err != nil {
			t.Fatalf("GeneratePresignedURL error: %v", err)
		}
		fmt.Println("Direct PUT presigned URL:", url)
	})

	// multipart presigned URL per part
	var uploadID *string
	t.Run("multipart-init", func(t *testing.T) {
		in := &CreateMultipartUploadInput{StoragePath: &path}
		upID, err := s3c.CreateMultipartUpload(ctx, in)
		if err != nil {
			t.Fatalf("CreateMultipartUpload error: %v", err)
		}
		uploadID = upID
	})

	parts := []int{1, 2}
	for _, num := range parts {
		n := num
		t.Run(fmt.Sprintf("multipart-presigned-part-%d", n), func(t *testing.T) {
			if uploadID == nil || *uploadID == "" {
				t.Fatalf("uploadID empty")
			}
			in := &GeneratePresignedURLInput{
				Method:      &method,
				StoragePath: &path,
				UploadID:    uploadID,
				PartNumber:  &n,
			}
			u, err := s3c.GeneratePresignedURL(ctx, in)
			if err != nil {
				t.Fatalf("GeneratePresignedURL part %d error: %v", n, err)
			}
			if u == nil || *u == "" {
				t.Fatalf("GeneratePresignedURL part %d returned empty URL", n)
			}
			fmt.Println("Multipart presigned URL:", *u, "partNum:", n)
		})
	}
}
