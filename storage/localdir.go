package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
)

// LocalStorage .
type LocalStorage struct {
	cfg config.LocalStorageConfig

	Dir string
}

// NewLocalStorage .
func NewLocalStorage(cfg config.LocalStorageConfig) (*LocalStorage, error) {
	ls := &LocalStorage{
		cfg: cfg,
		Dir: cfg.Dir,
	}

	if err := os.MkdirAll(ls.cfg.Dir, 0755); err != nil {
		return nil, err
	}
	return ls, nil
}

func (ls *LocalStorage) Save(ctx context.Context, fi *FileInfo, r io.Reader) error {
	fi.Filename = filepath.Clean(fi.Filename)
	fpath := filepath.Join(ls.Dir, fi.Filename)
	dir := filepath.Dir(fpath)

	if _, err := os.Stat(dir); err != nil {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			logs.Errorf("[local_storage] mkdir %s failed, %s", dir, err)
			return err
		}
	}

	f, err := os.Create(fpath)
	if err != nil {
		logs.Errorf("[local_storage] create file %s failed, %s", fpath, err)
		return err
	}
	fi.Size, err = io.Copy(f, r)
	if err != nil {
		logs.Errorf("[local_storage] write file %s failed, %s", fpath, err)
		return err
	}
	return nil
}
func (ls *LocalStorage) GetPublicURL(storagePath string, _ bool) string {
	// TODO: support custom domain
	return storagePath
}

func (ls *LocalStorage) GetPresignedURL(storagePath string) (string, error) {
	return "", nil
}
