package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"

	storage "github.com/ygpkg/yg-go/storage/v2"
)

func init() {
	storage.Register("local", func(cfg config.StorageConfig) (storage.Storager, error) {
		if cfg.Local == nil {
			return nil, fmt.Errorf("local config is nil")
		}
		return NewLocalStorage(*cfg.Local)
	})
}

var _ storage.Storager = (*LocalStorage)(nil)

type LocalStorage struct {
	cfg config.LocalStorageConfig
	Dir string
}

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

func (ls *LocalStorage) Save(ctx context.Context, fi *storage.FileInfo, r io.Reader) error {
	fi.StoragePath = filepath.Clean(fi.StoragePath)
	fpath := filepath.Join(ls.Dir, fi.StoragePath)
	dir := filepath.Dir(fpath)

	if _, err := os.Stat(dir); err != nil {
		if err := os.MkdirAll(dir, 0755); err != nil {
			logs.Errorf("[local_storage] mkdir %s failed: %s", dir, err)
			return err
		}
	}

	f, err := os.Create(fpath)
	if err != nil {
		logs.Errorf("[local_storage] create file %s failed: %s", fpath, err)
		return err
	}
	fi.Size, err = io.Copy(f, r)
	if err != nil {
		logs.Errorf("[local_storage] write file %s failed: %s", fpath, err)
		return err
	}
	return nil
}

func (ls *LocalStorage) GetPublicURL(storagePath string, _ bool) string {
	return fmt.Sprintf("%s/public.src?p=%s", ls.cfg.PublicPrefix, storagePath)
}

func (ls *LocalStorage) GetPresignedURL(method, storagePath string) (string, error) {
	return "", nil
}

func (ls *LocalStorage) ReadFile(storagePath string) (io.ReadCloser, error) {
	storagePath = filepath.Clean(storagePath)
	fpath := filepath.Join(ls.Dir, storagePath)
	if _, err := os.Stat(fpath); err != nil {
		logs.Errorf("[local_storage] file %s does not exist", fpath)
		return nil, err
	}
	file, err := os.Open(fpath)
	if err != nil {
		logs.Errorf("[local_storage] open file %s failed: %s", fpath, err)
		return nil, err
	}
	return file, nil
}

func (ls *LocalStorage) DeleteFile(storagePath string) error {
	storagePath = filepath.Clean(storagePath)
	fpath := filepath.Join(ls.Dir, storagePath)
	if _, err := os.Stat(fpath); err != nil {
		logs.Errorf("[local_storage] file %s does not exist", fpath)
		return err
	}
	if err := os.Remove(fpath); err != nil {
		logs.Errorf("[local_storage] delete file %s failed: %s", fpath, err)
		return err
	}
	return nil
}

func (ls *LocalStorage) CopyDir(storagePath, dest string) error {
	storagePath = filepath.Clean(storagePath)
	dest = filepath.Clean(dest)
	srcPath := filepath.Join(ls.Dir, storagePath)
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		logs.Errorf("[local_storage] source path %s does not exist", srcPath)
		return err
	}
	destPath := filepath.Join(ls.Dir, dest)
	if srcInfo.IsDir() {
		return ls.copyDirectory(srcPath, destPath)
	}
	return ls.copyFile(srcPath, destPath)
}

func (ls *LocalStorage) UploadDirectory(localDirPath, destDir string) ([]string, error) {
	return nil, fmt.Errorf("UploadDirectory not implemented for LocalStorage")
}

func (ls *LocalStorage) copyFile(srcPath, destPath string) error {
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, os.ModePerm); err != nil {
		logs.Errorf("[local_storage] create destination directory %s failed: %s", destDir, err)
		return err
	}
	srcFile, err := os.Open(srcPath)
	if err != nil {
		logs.Errorf("[local_storage] open source file %s failed: %s", srcPath, err)
		return err
	}
	defer srcFile.Close()
	destFile, err := os.Create(destPath)
	if err != nil {
		logs.Errorf("[local_storage] create destination file %s failed: %s", destPath, err)
		return err
	}
	defer destFile.Close()
	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		logs.Errorf("[local_storage] copy file %s to %s failed: %s", srcPath, destPath, err)
		return err
	}
	return nil
}

func (ls *LocalStorage) copyDirectory(srcPath, destPath string) error {
	if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
		logs.Errorf("[local_storage] create destination directory %s failed: %s", destPath, err)
		return err
	}
	entries, err := os.ReadDir(srcPath)
	if err != nil {
		logs.Errorf("[local_storage] read source directory %s failed: %s", srcPath, err)
		return err
	}
	for _, entry := range entries {
		srcEntryPath := filepath.Join(srcPath, entry.Name())
		destEntryPath := filepath.Join(destPath, entry.Name())
		if entry.IsDir() {
			if err := ls.copyDirectory(srcEntryPath, destEntryPath); err != nil {
				return err
			}
		} else {
			if err := ls.copyFile(srcEntryPath, destEntryPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *LocalStorage) CreateMultipartUpload(ctx context.Context, in *storage.CreateMultipartUploadInput) (*string, error) {
	return nil, fmt.Errorf("multipart upload not supported for LocalStorage")
}
func (l *LocalStorage) GeneratePresignedURL(ctx context.Context, in *storage.GeneratePresignedURLInput) (*string, error) {
	return nil, fmt.Errorf("presigned part URL not supported for LocalStorage")
}
func (l *LocalStorage) UploadPart(ctx context.Context, in *storage.UploadPartInput) (*string, error) {
	return nil, fmt.Errorf("multipart upload not supported for LocalStorage")
}
func (l *LocalStorage) CompleteMultipartUpload(ctx context.Context, in *storage.CompleteMultipartUploadInput) error {
	return fmt.Errorf("multipart upload not supported for LocalStorage")
}
func (l *LocalStorage) AbortMultipartUpload(ctx context.Context, in *storage.AbortMultipartUploadInput) error {
	return fmt.Errorf("multipart upload not supported for LocalStorage")
}
