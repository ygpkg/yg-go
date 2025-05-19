package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

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
		// ContentType: fi.FileExt,
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
func (mfs *MinFs) GetPresignedURL(method, storagePath string) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := mfs.client.PresignedGetObject(mfs.ctx, mfs.mfsCfg.Bucket, storagePath, mfs.opt.PresignedTimeout, reqParams)
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

// CopyDir 复制文件或文件夹
func (mfs *MinFs) CopyDir(storagePath, dest string) error {
	if storagePath == "" {
		return fmt.Errorf("source storage path is empty")
	}
	if dest == "" {
		return fmt.Errorf("destination storage path is empty")
	}

	// 检查源路径是文件还是文件夹
	isDir, err := mfs.isDirectory(storagePath)
	if err != nil {
		logs.Errorf("minio check source path error: %v", err)
		return err
	}

	if isDir {
		// 复制文件夹
		return mfs.copyDirectory(storagePath, dest)
	} else {
		// 复制文件
		return mfs.copyObject(storagePath, dest)
	}
}

// isDirectory 检查路径是否为文件夹
func (mfs *MinFs) isDirectory(storagePath string) (bool, error) {
	opts := minio.StatObjectOptions{}
	_, err := mfs.client.StatObject(mfs.ctx, mfs.mfsCfg.Bucket, storagePath, opts)
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			// 如果路径不存在，则认为是文件夹
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// copyObject 复制文件
func (mfs *MinFs) copyObject(storagePath, dest string) error {
	src := minio.CopySrcOptions{
		Bucket: mfs.mfsCfg.Bucket,
		Object: storagePath,
	}
	dst := minio.CopyDestOptions{
		Bucket: mfs.mfsCfg.Bucket,
		Object: dest,
	}

	_, err := mfs.client.CopyObject(mfs.ctx, dst, src)
	if err != nil {
		logs.Errorf("minio copy object error: %v", err)
		return err
	}

	return nil
}

// copyDirectory 复制文件夹
func (mfs *MinFs) copyDirectory(storagePath, dest string) error {
	// 列出源文件夹中的所有对象
	objectsCh := mfs.client.ListObjects(mfs.ctx, mfs.mfsCfg.Bucket, minio.ListObjectsOptions{
		Prefix:    storagePath,
		Recursive: true,
	})

	for obj := range objectsCh {
		if obj.Err != nil {
			logs.Errorf("minio list objects error: %v", obj.Err)
			return obj.Err
		}

		// 计算目标路径
		relativePath := strings.TrimPrefix(obj.Key, storagePath)
		targetPath := path.Join(dest, relativePath)

		// 复制对象
		err := mfs.copyObject(obj.Key, targetPath)
		if err != nil {
			return err
		}
	}

	return nil
}
