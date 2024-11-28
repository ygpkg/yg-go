package storage

import (
	"context"
	"fmt"
	"io"
	"sync"

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
	GetPresignedURL(storagePath string) (string, error)
	ReadFile(fi *FileInfo) (io.Reader, error)
	// Stat(name string) (*FileInfo, error)
	DeleteFile(fi *FileInfo) error
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
	} else {
		return nil, fmt.Errorf("not found useful remote storage config")
	}
	if err != nil {
		logs.Errorf("new storage error: %v", err)
		return nil, err
	}
	return s, nil
}
