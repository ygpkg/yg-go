package storage

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
	"github.com/ygpkg/yg-go/config"
)

func TestMultipartUpload(t *testing.T) {
	if true {
		return
	}
	cosCfg := config.TencentCOSConfig{
		Bucket: "test-borui-1251908240",
		TencentConfig: config.TencentConfig{
			SecretID:  os.Getenv("TCOS_SECRET_ID"),
			SecretKey: os.Getenv("TCOS_SECRET_KEY"),
			Region:    "ap-beijing",
		},
	}

	if cosCfg.SecretID == "" || cosCfg.SecretKey == "" {
		t.Skip("skip test, no tencent cos config")
		return
	}

	tcos, err := NewTencentCos(cosCfg, config.StorageOption{})
	if err != nil {
		t.Fatal(err)
	}

	testFile := "/test/test1.txt"
	ctx := context.Background()
	initOpt := &cos.InitiateMultipartUploadOptions{}
	initRst, _, err := tcos.client.Object.InitiateMultipartUpload(ctx, testFile, initOpt)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		buf := new(bytes.Buffer)
		buf.WriteString(strconv.Itoa(i + 1))
		for k := 0; k < 1024*256; k++ {
			buf.WriteString("tttest")
		}
		buf.WriteString("\n\n\n")
		_, err := tcos.client.Object.UploadPart(ctx, testFile, initRst.UploadID, i+1, buf, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("upload part %d", i+1)

		listRst, _, err := tcos.client.Object.ListParts(ctx, testFile, initRst.UploadID, nil)
		if err != nil {
			t.Fatal(err)
		}
		parts := make([]int, 0, len(listRst.Parts))
		for _, part := range listRst.Parts {
			parts = append(parts, part.PartNumber)
		}
		t.Logf("list parts %d, %+v", len(listRst.Parts), parts)
	}

	listRst, _, err := tcos.client.Object.ListParts(ctx, testFile, initRst.UploadID, nil)
	if err != nil {
		t.Fatal(err)
	}
	compOpt := &cos.CompleteMultipartUploadOptions{
		Parts: listRst.Parts,
	}
	compRst, _, err := tcos.client.Object.CompleteMultipartUpload(ctx, testFile, initRst.UploadID, compOpt)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("complete upload %+v", compRst)
}

func TestSimpleUpload(t *testing.T) {
	cosCfg := config.TencentCOSConfig{
		Bucket: "test-ccnerf-1251908240",
		TencentConfig: config.TencentConfig{
			SecretID:  os.Getenv("TCOS_SECRET_ID"),
			SecretKey: os.Getenv("TCOS_SECRET_KEY"),
			Region:    "ap-beijing",
		},
	}
	if cosCfg.SecretID == "" || cosCfg.SecretKey == "" {
		t.Skip("skip test, no tencent cos config")
		return
	}

	tcos, err := NewTencentCos(cosCfg, config.StorageOption{})
	if err != nil {
		t.Fatal(err)
	}

	testFile := "/mnt/f/code/go/src/br/borui-api/bf4d-f0cf7646fdacc4d68595b34350a8fda8.jpg"
	f, err := os.Open(testFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tcos.Save(context.Background(), &FileInfo{StoragePath: "/test/test1.jpg"}, f)

}

func TestGetPresignedURL(t *testing.T) {
	cosCfg := config.TencentCOSConfig{
		Bucket: "test-ccnerf-1251908240",
		TencentConfig: config.TencentConfig{
			SecretID:  os.Getenv("TCOS_SECRET_ID"),
			SecretKey: os.Getenv("TCOS_SECRET_KEY"),
			Region:    "ap-beijing",
		},
	}
	if cosCfg.SecretID == "" || cosCfg.SecretKey == "" {
		t.Skip("skip test, no tencent cos config")
		return
	}

	tcos, err := NewTencentCos(cosCfg, config.StorageOption{PresignedTimeout: time.Second * 30})
	if err != nil {
		t.Fatal(err)
	}

	u := tcos.GetPublicURL("/test/test1.jpg", true)
	t.Logf("url: %s", u)
}
