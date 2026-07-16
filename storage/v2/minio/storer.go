package minio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	minioClient "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"

	storagev2 "github.com/ygpkg/yg-go/storage/v2"
)

func init() {
	storagev2.Register("minio", func(cfg config.StorageConfig) (storagev2.Storager, error) {
		if cfg.Minoss == nil {
			return nil, fmt.Errorf("minio config is nil")
		}
		return NewMinFs(*cfg.Minoss, cfg.StorageOption)
	})
}

var _ storagev2.Storager = (*MinFs)(nil)

type MinFs struct {
	opt    config.StorageOption
	mfsCfg config.MinossConfig
	client *minioClient.Client
	ctx    context.Context
}

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
	mc.client, err = minioClient.New(cfg.EndPoint, &minioClient.Options{
		Creds: credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
	})
	if err != nil {
		return nil, err
	}
	isE, err := mc.client.BucketExists(mc.ctx, cfg.Bucket)
	if err != nil {
		return nil, err
	}
	if !isE {
		return nil, errors.New("bucket not exists")
	}
	return mc, nil
}

func (mfs *MinFs) Save(ctx context.Context, fi *storagev2.FileInfo, r io.Reader) error {
	if fi.StoragePath == "" {
		return fmt.Errorf("storage path is empty")
	}
	if r == nil {
		return fmt.Errorf("reader is empty")
	}
	_, err := mfs.client.PutObject(mfs.ctx, mfs.mfsCfg.Bucket, fi.StoragePath, r, fi.Size, minioClient.PutObjectOptions{
		ContentType: mime.TypeByExtension(fi.FileExt),
	})
	return err
}

func (mfs *MinFs) GetPublicURL(storagePath string, _ bool) string {
	return storagePath
}

func (mfs *MinFs) GetPresignedURL(method, storagePath string) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := mfs.client.PresignedGetObject(mfs.ctx, mfs.mfsCfg.Bucket, storagePath, mfs.opt.PresignedTimeout, reqParams)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

func (mfs *MinFs) ReadFile(storagePath string) (io.ReadCloser, error) {
	if storagePath == "" {
		return nil, fmt.Errorf("storage path is empty")
	}
	obj, err := mfs.client.GetObject(mfs.ctx, mfs.mfsCfg.Bucket, storagePath, minioClient.GetObjectOptions{})
	if err != nil {
		logs.Errorf("minoss get object error: %v", err)
		return nil, err
	}
	return obj, nil
}

func (mfs *MinFs) DeleteFile(storagePath string) error {
	if storagePath == "" {
		return fmt.Errorf("storage path is empty")
	}
	err := mfs.client.RemoveObject(mfs.ctx, mfs.mfsCfg.Bucket, storagePath, minioClient.RemoveObjectOptions{
		GovernanceBypass: true,
	})
	if err != nil {
		logs.Errorf("minoss delete object error: %v", err)
	}
	return err
}

func (mfs *MinFs) CopyDir(storagePath, dest string) error {
	if storagePath == "" {
		return fmt.Errorf("source storage path is empty")
	}
	if dest == "" {
		return fmt.Errorf("destination storage path is empty")
	}
	isDir, err := mfs.isDirectory(storagePath)
	if err != nil {
		logs.Errorf("minio check source path error: %v", err)
		return err
	}
	if isDir {
		return mfs.copyDirectory(storagePath, dest)
	}
	return mfs.copyObject(storagePath, dest)
}

func (mfs *MinFs) UploadDirectory(localDirPath, destDir string) ([]string, error) {
	return nil, fmt.Errorf("UploadDirectory not implemented for MinFs")
}

func (mfs *MinFs) isDirectory(storagePath string) (bool, error) {
	opts := minioClient.StatObjectOptions{}
	_, err := mfs.client.StatObject(mfs.ctx, mfs.mfsCfg.Bucket, storagePath, opts)
	if err != nil {
		if minioClient.ToErrorResponse(err).Code == "NoSuchKey" {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func (mfs *MinFs) copyObject(storagePath, dest string) error {
	src := minioClient.CopySrcOptions{
		Bucket: mfs.mfsCfg.Bucket,
		Object: storagePath,
	}
	dst := minioClient.CopyDestOptions{
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

func (mfs *MinFs) copyDirectory(storagePath, dest string) error {
	objectsCh := mfs.client.ListObjects(mfs.ctx, mfs.mfsCfg.Bucket, minioClient.ListObjectsOptions{
		Prefix:    storagePath,
		Recursive: true,
	})
	for obj := range objectsCh {
		if obj.Err != nil {
			logs.Errorf("minio list objects error: %v", obj.Err)
			return obj.Err
		}
		relativePath := strings.TrimPrefix(obj.Key, storagePath)
		targetPath := path.Join(dest, relativePath)
		if err := mfs.copyObject(obj.Key, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func (mfs *MinFs) CreateMultipartUpload(ctx context.Context, in *storagev2.CreateMultipartUploadInput) (*string, error) {
	if in == nil || in.StoragePath == nil || *in.StoragePath == "" {
		return nil, fmt.Errorf("storage path is empty")
	}
	core := minioClient.Core{Client: mfs.client}
	uploadID, err := core.NewMultipartUpload(ctx, mfs.mfsCfg.Bucket, *in.StoragePath, minioClient.PutObjectOptions{})
	if err != nil {
		logs.Errorf("minoss initiate multipart upload error: %v", err)
		return nil, err
	}
	return &uploadID, nil
}

func (mfs *MinFs) GeneratePresignedURL(ctx context.Context, in *storagev2.GeneratePresignedURLInput) (*string, error) {
	return nil, fmt.Errorf("presigned part URL not supported for MinIO")
}

func (mfs *MinFs) UploadPart(ctx context.Context, in *storagev2.UploadPartInput) (*string, error) {
	if in == nil || in.StoragePath == nil || *in.StoragePath == "" {
		return nil, fmt.Errorf("storage path is empty")
	}
	if in.UploadID == nil || in.PartNumber == nil {
		return nil, fmt.Errorf("uploadID or partNumber is nil")
	}
	if in.Data == nil {
		return nil, fmt.Errorf("reader is empty")
	}
	core := minioClient.Core{Client: mfs.client}
	objPart, err := core.PutObjectPart(ctx, mfs.mfsCfg.Bucket, *in.StoragePath, *in.UploadID, *in.PartNumber, in.Data, -1, minioClient.PutObjectPartOptions{})
	if err != nil {
		logs.Errorf("minoss upload part error: %v", err)
		return nil, err
	}
	return &objPart.ETag, nil
}

func (mfs *MinFs) CompleteMultipartUpload(ctx context.Context, in *storagev2.CompleteMultipartUploadInput) error {
	if in == nil || in.StoragePath == nil || in.UploadID == nil || in.Parts == nil {
		return fmt.Errorf("storagePath, uploadID or parts is nil")
	}
	core := minioClient.Core{Client: mfs.client}
	compParts := make([]minioClient.CompletePart, 0, len(in.Parts.Parts))
	for _, p := range in.Parts.Parts {
		compParts = append(compParts, minioClient.CompletePart{ETag: aws.ToString(p.ETag), PartNumber: int(aws.ToInt32(p.PartNumber))})
	}
	_, err := core.CompleteMultipartUpload(ctx, mfs.mfsCfg.Bucket, *in.StoragePath, *in.UploadID, compParts, minioClient.PutObjectOptions{})
	if err != nil {
		logs.Errorf("minoss complete multipart upload error: %v", err)
	}
	return err
}

func (mfs *MinFs) AbortMultipartUpload(ctx context.Context, in *storagev2.AbortMultipartUploadInput) error {
	if in == nil || in.StoragePath == nil || in.UploadID == nil {
		return fmt.Errorf("storagePath or uploadID is nil")
	}
	core := minioClient.Core{Client: mfs.client}
	err := core.AbortMultipartUpload(ctx, mfs.mfsCfg.Bucket, *in.StoragePath, *in.UploadID)
	if err != nil {
		logs.Errorf("minoss abort multipart upload error: %v", err)
	}
	return err
}
