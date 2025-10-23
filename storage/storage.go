package storage

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
	"gorm.io/gorm"
)

var storagerMap = new(sync.Map)

const (
	TableNameFileInfo = "core_upload_files"
	TableNameTempFile = "core_upload_files_tmp"
)

const (
	// SettingPrefix 配置前缀
	SettingPrefix = "cos-"
)

// Storager .
type Storager interface {
	Save(ctx context.Context, fi *FileInfo, data io.Reader) error
	GetPublicURL(storagePath string, temp bool) string
	GetPresignedURL(method, storagePath string) (string, error)
	ReadFile(storagePath string) (io.ReadCloser, error)
	// Stat(name string) (*FileInfo, error)
	DeleteFile(storagePath string) error
	CopyDir(storagePath, dest string) error
	UploadDirectory(localDirPath, destDir string) ([]string, error)

	// CreateMultipartUpload 创建分片上传
	CreateMultipartUpload(ctx context.Context, in *CreateMultipartUploadInput) (*string, error)
	// GeneratePresignedPartURL 生成上传预签名URL
	GeneratePresignedURL(ctx context.Context, in *GeneratePresignedURLInput) (*string, error)
	// UploadPart 上传分片
	UploadPart(ctx context.Context, in *UploadPartInput) (*string, error)
	// CompleteMultipartUpload 完成分片上传
	CompleteMultipartUpload(ctx context.Context, in *CompleteMultipartUploadInput) error
	// AbortMultipartUpload 取消分片上传
	AbortMultipartUpload(ctx context.Context, in *AbortMultipartUploadInput) error
}

// CreateMultipartUploadInput 请求对象
type CreateMultipartUploadInput struct {
	Bucket      *string
	StoragePath *string
	ContentType *string
}

// GeneratePresignedURLInput 请求对象
type GeneratePresignedURLInput struct {
	Method        *string
	Bucket        *string
	StoragePath   *string
	UploadID      *string
	PartNumber    *int
	ContentType   *string
	ContentLength *int64
	ContentMD5    *string
}

// UploadPartInput 请求对象
type UploadPartInput struct {
	Bucket      *string
	StoragePath *string
	UploadID    *string
	PartNumber  *int
	Data        io.Reader
}

// CompleteMultipartUploadInput 请求对象
type CompleteMultipartUploadInput struct {
	Bucket      *string
	StoragePath *string
	UploadID    *string
	Parts       *types.CompletedMultipartUpload
}

// AbortMultipartUploadInput 请求对象
type AbortMultipartUploadInput struct {
	Bucket      *string
	StoragePath *string
	UploadID    *string
}

// InitDB .
func InitDB(db *gorm.DB) error {
	return dbtools.InitModel(db, &FileInfo{}, &TempFile{})
}

// UploadFile 上传文件
func UploadFile(ctx context.Context, fi *FileInfo, r io.Reader) error {
	s, err := LoadStorager(fi.Purpose)
	if err != nil {
		return err
	}

	return s.Save(ctx, fi, r)
}

// LoadStorager 获取存储器
func LoadStorager(purpose string) (Storager, error) {
	if s, ok := storagerMap.Load(purpose); ok {
		return s.(Storager), nil
	}
	s, err := NewStorage(purpose)
	if err != nil {
		return nil, err
	}
	storagerMap.Store(purpose, s)
	return s, nil
}

// NewStorage .
func NewStorage(purpose string) (Storager, error) {
	var (
		cfg config.StorageConfig
		s   Storager
		err error
		key = SettingPrefix + purpose
	)
	err = settings.GetYaml(settings.SettingGroupCore, key, &cfg)
	if err != nil {
		logs.Errorf("get storage config error: %v", err)
		return nil, err
	}

	if cfg.Local != nil {
		s, err = NewLocalStorage(*cfg.Local)
	} else if cfg.Tencent != nil {
		s, err = NewTencentCos(*cfg.Tencent, cfg.StorageOption)
	} else if cfg.Minoss != nil {
		s, err = NewMinFs(*cfg.Minoss, cfg.StorageOption)
	} else if cfg.S3 != nil {
		s, err = NewS3Fs(*cfg.S3, cfg.StorageOption)
	} else {
		return nil, fmt.Errorf("not found useful remote storage config")
	}
	if err != nil {
		logs.Errorf("new storage error: %v", err)
		return nil, err
	}
	return s, nil
}

// NewStorageWithCfg New a storage with cfg
func NewStorageWithCfg(cfg config.StorageConfig) (Storager, error) {
	var (
		s   Storager
		err error
	)

	if cfg.Local != nil {
		s, err = NewLocalStorage(*cfg.Local)
	} else if cfg.Tencent != nil {
		s, err = NewTencentCos(*cfg.Tencent, cfg.StorageOption)
	} else if cfg.Minoss != nil {
		s, err = NewMinFs(*cfg.Minoss, cfg.StorageOption)
	} else if cfg.S3 != nil {
		s, err = NewS3Fs(*cfg.S3, cfg.StorageOption)
	} else {
		return nil, fmt.Errorf("not found useful remote storage config")
	}
	if err != nil {
		logs.Errorf("new storage error: %v", err)
		return nil, err
	}
	return s, nil
}
