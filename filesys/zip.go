package filesys

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
)

func Unzip(zipFile, dest string) error {
	reader, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		destPath := filepath.Join(dest, file.Name)

		if file.FileInfo().IsDir() {
			err = os.MkdirAll(destPath, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			err = os.MkdirAll(filepath.Dir(destPath), os.ModePerm)
			if err != nil {
				return err
			}

			destFile, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer destFile.Close()

			srcFile, err := file.Open()
			if err != nil {
				return err
			}
			defer srcFile.Close()

			_, err = io.Copy(destFile, srcFile)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
