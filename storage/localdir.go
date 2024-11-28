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
	fi.StoragePath = filepath.Clean(fi.StoragePath)
	fpath := filepath.Join(ls.Dir, fi.StoragePath)
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

// ReadFile 获取文件内容
func (ls *LocalStorage) ReadFile(storagePath string) (io.ReadCloser, error) {
	storagePath = filepath.Clean(storagePath)
	// 构建文件路径
	fpath := filepath.Join(ls.Dir, storagePath)

	// 检查文件是否存在
	if _, err := os.Stat(fpath); err != nil {
		logs.Errorf("[local_storage] file %s does not exist", fpath)
		return nil, err
	}
	// 打开文件
	file, err := os.Open(fpath)
	if err != nil {
		logs.Errorf("[local_storage] open file %s failed, %s", fpath, err)
		return nil, err
	}

	// 返回文件的 Reader
	return file, nil
}

// DeleteFile 删除文件
func (ls *LocalStorage) DeleteFile(storagePath string) error {
	storagePath = filepath.Clean(storagePath)
	// 构建文件的完整路径
	fpath := filepath.Join(ls.Dir, storagePath)
	// 检查文件是否存在
	if _, err := os.Stat(fpath); err != nil {
		logs.Errorf("[local_storage] file %s does not exist", fpath)
		return err
	}
	// 删除文件
	err := os.Remove(fpath)
	if err != nil {
		logs.Errorf("[local_storage] delete file %s failed, %s", fpath, err)
		return err
	}

	return nil
}
