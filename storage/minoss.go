package storage

import (
	minio "github.com/minio/minio-go/v7"
	"github.com/ygpkg/yg-go/config"
)

type MinFs struct {
	opt    config.StorageOption
	cosCfg config.MinossConfig
	client *minio.Client
}
