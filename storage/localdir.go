package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
)

var _ Storager = (*LocalStorage)(nil)

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
	return fmt.Sprintf("%s/public.src?p=%s", ls.cfg.PublicPrefix, storagePath)
}

func (ls *LocalStorage) GetPresignedURL(method, storagePath string) (string, error) {
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

// CopyDir 复制文件或文件夹
func (ls *LocalStorage) CopyDir(storagePath, dest string) error {
	storagePath = filepath.Clean(storagePath)
	dest = filepath.Clean(dest)

	// 构建源路径的完整路径
	srcPath := filepath.Join(ls.Dir, storagePath)

	// 检查源路径是否存在
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		logs.Errorf("[local_storage] source path %s does not exist", srcPath)
		return err
	}

	// 构建目标路径的完整路径
	destPath := filepath.Join(ls.Dir, dest)

	// 如果源路径是文件夹，则递归复制文件夹
	if srcInfo.IsDir() {
		return ls.copyDirectory(srcPath, destPath)
	}

	// 如果源路径是文件，则复制文件
	return ls.copyFile(srcPath, destPath)
}
func (ls *LocalStorage) UploadDirectory(localDirPath, destDir string) ([]string, error) {
	return nil, fmt.Errorf("UploadDirectory not implemented for LocalStorage")
}

// copyFile 复制文件
func (ls *LocalStorage) copyFile(srcPath, destPath string) error {
	// 创建目标文件的目录
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, os.ModePerm); err != nil {
		logs.Errorf("[local_storage] create destination directory %s failed, %s", destDir, err)
		return err
	}

	// 打开源文件
	srcFile, err := os.Open(srcPath)
	if err != nil {
		logs.Errorf("[local_storage] open source file %s failed, %s", srcPath, err)
		return err
	}
	defer srcFile.Close()

	// 创建目标文件
	destFile, err := os.Create(destPath)
	if err != nil {
		logs.Errorf("[local_storage] create destination file %s failed, %s", destPath, err)
		return err
	}
	defer destFile.Close()

	// 复制文件内容
	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		logs.Errorf("[local_storage] copy file %s to %s failed, %s", srcPath, destPath, err)
		return err
	}

	return nil
}

// copyDirectory 复制文件夹
func (ls *LocalStorage) copyDirectory(srcPath, destPath string) error {
	// 创建目标文件夹
	if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
		logs.Errorf("[local_storage] create destination directory %s failed, %s", destPath, err)
		return err
	}

	// 遍历源文件夹中的所有文件和子文件夹
	entries, err := os.ReadDir(srcPath)
	if err != nil {
		logs.Errorf("[local_storage] read source directory %s failed, %s", srcPath, err)
		return err
	}

	for _, entry := range entries {
		srcEntryPath := filepath.Join(srcPath, entry.Name())
		destEntryPath := filepath.Join(destPath, entry.Name())

		if entry.IsDir() {
			// 递归复制子文件夹
			if err := ls.copyDirectory(srcEntryPath, destEntryPath); err != nil {
				return err
			}
		} else {
			// 复制文件
			if err := ls.copyFile(srcEntryPath, destEntryPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func (l *LocalStorage) CreateMultipartUpload(ctx context.Context, in *CreateMultipartUploadInput) (*string, error) {
	return nil, fmt.Errorf("multipart upload not supported for LocalStorage")
}
func (l *LocalStorage) GeneratePresignedURL(ctx context.Context, in *GeneratePresignedURLInput) (*string, error) {
	return nil, fmt.Errorf("presigned part URL not supported for LocalStorage")
}
func (l *LocalStorage) UploadPart(ctx context.Context, in *UploadPartInput) (*string, error) {
	return nil, fmt.Errorf("multipart upload not supported for LocalStorage")
}
func (l *LocalStorage) CompleteMultipartUpload(ctx context.Context, in *CompleteMultipartUploadInput) error {
	return fmt.Errorf("multipart upload not supported for LocalStorage")
}
func (l *LocalStorage) AbortMultipartUpload(ctx context.Context, in *AbortMultipartUploadInput) error {
	return fmt.Errorf("multipart upload not supported for LocalStorage")
}
