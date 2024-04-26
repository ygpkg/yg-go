package filesys

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ygpkg/yg-go/utils/random"
)

// CreateRandomFile 创建随机文件
func CreateRandomFile(dir, prefix string, suffix string) (string, *os.File, error) {
	randstr := random.Number(7)
	filename := fmt.Sprintf("%s%s%s", prefix, randstr, suffix)
	filepath := filepath.Join(dir, filename)

	f, err := os.Create(filepath)
	if err != nil {
		return "", nil, err
	}

	return filepath, f, nil
}

// MoveFile 移动文件
func MoveFile(sourcePath, destPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("Couldn't open source file: %s", err)
	}
	defer inputFile.Close()

	outputFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("Couldn't open dest file: %s", err)
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		return fmt.Errorf("Writing to output file failed: %s", err)
	}
	// The copy was successful, so now delete the original file
	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("Failed removing original file: %s", err)
	}
	return nil
}
