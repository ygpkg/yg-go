package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
)

// MinFs .
type MinFs struct {
	opt    config.StorageOption
	mfsCfg config.MinossConfig
	client *minio.Client
	ctx    context.Context
}

// NewMinFs 初始化MinFs
func NewMinFs(cfg config.MinossConfig, opt config.StorageOption) (*MinFs, error) {
	var err error
	if cfg.Bucket == "" {
		return nil, errors.New("configuration bucket error")
	}

	mc := &MinFs{
		opt:    opt,
		mfsCfg: cfg,
		ctx:    context.Background(),
	}
	mc.client, err = minio.New(cfg.EndPoint, &minio.Options{
		Creds: credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		// Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, err
	}
	// set bucket
	isE, err := mc.client.BucketExists(mc.ctx, cfg.Bucket)
	if err != nil {
		return nil, err
	}
	if !isE {
		return nil, errors.New("bucket not exists")
	}

	return mc, nil
}

// Save 保存文件
func (mfs *MinFs) Save(ctx context.Context, fi *FileInfo, r io.Reader) error {
	if fi.StoragePath == "" {
		return fmt.Errorf("storage path is empty")
	}
	if r == nil {
		return fmt.Errorf("reader is empty")
	}
	// 上传一条记录
	_, err := mfs.client.PutObject(mfs.ctx, mfs.mfsCfg.Bucket, fi.StoragePath, r, fi.Size, minio.PutObjectOptions{
		ContentType: fi.FileExt,
	})

	if err != nil {
		return err
	}

	return nil
}

// GetPublicURL 获取公共URL
func (mfs *MinFs) GetPublicURL(storagePath string, _ bool) string {
	// TODO: support custom domain
	return storagePath
}

// GetPresignedURL 获取预签名URL
func (mfs *MinFs) GetPresignedURL(storagePath string) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := mfs.client.PresignedGetObject(mfs.ctx, mfs.mfsCfg.Bucket, storagePath, time.Hour*24, reqParams)
	if err != nil {
		// 如果生成预签名URL失败，返回原始存储路径
		return "", err
	}

	return presignedURL.String(), nil
}

// ReadFile 获取文件内容
func (mfs *MinFs) ReadFile(storagePath string) (io.ReadCloser, error) {
	if storagePath == "" {
		return nil, fmt.Errorf("storage path is empty")
	}

	obj, err := mfs.client.GetObject(mfs.ctx, mfs.mfsCfg.Bucket, storagePath, minio.GetObjectOptions{})
	if err != nil {
		logs.Errorf("minoss get object error: %v", err)
		return nil, err
	}
	return obj, nil
}

// DeleteFile 删除文件
func (mfs *MinFs) DeleteFile(storagePath string) error {
	if storagePath == "" {
		return fmt.Errorf("storage path is empty")
	}
	err := mfs.client.RemoveObject(mfs.ctx, mfs.mfsCfg.Bucket, storagePath, minio.RemoveObjectOptions{
		GovernanceBypass: true,
	})
	if err != nil {
		logs.Errorf("minoss delete object error: %v", err)
	}
	return err
}
