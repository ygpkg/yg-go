package storage

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
)

type Storager interface {
	Save(ctx context.Context, fi *FileInfo, data io.Reader) error
	GetPublicURL(storagePath string, temp bool) string
	GetPresignedURL(method, storagePath string) (string, error)
	ReadFile(storagePath string) (io.ReadCloser, error)
	DeleteFile(storagePath string) error
	CopyDir(storagePath, dest string) error
	UploadDirectory(localDirPath, destDir string) ([]string, error)
	CreateMultipartUpload(ctx context.Context, in *CreateMultipartUploadInput) (*string, error)
	GeneratePresignedURL(ctx context.Context, in *GeneratePresignedURLInput) (*string, error)
	UploadPart(ctx context.Context, in *UploadPartInput) (*string, error)
	CompleteMultipartUpload(ctx context.Context, in *CompleteMultipartUploadInput) error
	AbortMultipartUpload(ctx context.Context, in *AbortMultipartUploadInput) error
}

type FactoryFunc func(cfg config.StorageConfig) (Storager, error)

var (
	registry   = map[string]FactoryFunc{}
	registryMu sync.RWMutex
)

func Register(name string, fn FactoryFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = fn
}

var storagerMap = new(sync.Map)

const SettingPrefix = "cos-"

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

func NewStorage(purpose string) (Storager, error) {
	var cfg config.StorageConfig
	key := SettingPrefix + purpose
	err := settings.GetYaml(settings.SettingGroupCore, key, &cfg)
	if err != nil {
		logs.Errorf("get storage config error: %v", err)
		return nil, err
	}
	return NewStorageWithCfg(cfg)
}

func NewStorageWithCfg(cfg config.StorageConfig) (Storager, error) {
	var (
		kind string
	)
	if cfg.Local != nil {
		kind = "local"
	} else if cfg.Tencent != nil {
		kind = "tencos"
	} else if cfg.Minoss != nil {
		kind = "minio"
	} else if cfg.S3 != nil {
		kind = "s3"
	} else {
		return nil, fmt.Errorf("no storage config matched, registered drivers: %v", registeredNames())
	}

	registryMu.RLock()
	factory, ok := registry[kind]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("storage driver %q not registered, please import _ \"github.com/ygpkg/yg-go/storage/v2/%s\"", kind, kind)
	}

	s, err := factory(cfg)
	if err != nil {
		logs.Errorf("new storage %q error: %v", kind, err)
		return nil, err
	}
	return s, nil
}

func registeredNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}
