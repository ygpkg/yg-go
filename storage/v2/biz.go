package storage

import (
	"context"
	"io"
)

func UploadFile(ctx context.Context, fi *FileInfo, r io.Reader) error {
	s, err := LoadStorager(fi.Purpose)
	if err != nil {
		return err
	}
	return s.Save(ctx, fi, r)
}

func getNeedUploadPartNumbers(partCount int64, exiPartNumbers []int) []int {
	var need []int
	for i := 0; i < int(partCount); i++ {
		if !contains(exiPartNumbers, i) {
			need = append(need, i)
		}
	}
	return need
}

func contains(arr []int, val int) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}
