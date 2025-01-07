package weedfs

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/linxGnu/goseaweedfs"
	"github.com/ygpkg/yg-go/logs"
)

// WeedFileChunk weed 文件块
type WeedFileChunk struct {
	FileID string `json:"file_id"`
	Size   int64  `json:"size"`
}

// WeedFileEntry weed 文件
type WeedFileEntry struct {
	FullPath string          `json:"FullPath"`
	Mode     fs.FileMode     `json:"Mode"`
	Mime     string          `json:"Mime"`
	Md5      string          `json:"Md5"`
	FileSize int64           `json:"FileSize"`
	Chunks   []WeedFileChunk `json:"chunks"`
}

// WeedFileListResponse weed 文件列表
type WeedFileListResponse struct {
	Path    string           `json:"Path"`
	Entries []*WeedFileEntry `json:"Entries"`
}

// WalkWeedFunc 遍历 weed 文件夹
type WalkWeedFunc func(ctx context.Context, filer *goseaweedfs.Filer, pathStartWith string, ent *WeedFileEntry, err error) error

// WalkWeedDir 遍历 weed 文件夹
func WalkWeedDir(ctx context.Context, filer *goseaweedfs.Filer, rootDir string, wf WalkWeedFunc) error {
	var (
		lastFilename = ""
	)

	for {
		entries, err := ListWeedDirFiles(filer, rootDir, lastFilename)
		if err != nil {
			logs.Errorf("get weed dir files error: %v", err)
			return err
		}
		for _, ent := range entries {
			if err := wf(ctx, filer, rootDir, ent, nil); err != nil {
				return err
			}
			if ent.Mode.IsDir() {
				if err := WalkWeedDir(ctx, filer, ent.FullPath, wf); err != nil {
					return err
				}
			}
		}
		if len(entries) == 0 {
			break
		}
		lastFilename = filepath.Base(entries[len(entries)-1].FullPath)
	}
	return nil
}

// ListWeedDirFiles 获取 weed 文件夹下的文件列表
func ListWeedDirFiles(filer *goseaweedfs.Filer, rootDir string, lastFilename string) ([]*WeedFileEntry, error) {
	header := map[string]string{
		"Accept": "application/json",
	}
	args := map[string][]string{
		"limit": {"500"},
	}
	if lastFilename != "" {
		if strings.Contains(lastFilename, "/") {
			lastFilename = filepath.Base(lastFilename)
		}
		args["lastFileName"] = []string{lastFilename}
	}

	data, code, err := filer.Get(rootDir, args, header)
	if err != nil {
		logs.Errorf("weed filer get file error: %v", err)
		return nil, err
	}
	if code != 200 {
		logs.Errorf("weed filer get file error status code: %v", code)
		return nil, err
	}
	var resp WeedFileListResponse
	err = json.Unmarshal(data, &resp)
	if err != nil {
		logs.Errorf("weed filer get file unmarshal error: %v", err)
		return nil, err
	}
	return resp.Entries, nil
}

// ExistsFile weed 文件是否存在
func ExistsFile(filerurl, filename string) bool {
	resp, err := http.Head(filerurl + filename)
	if err != nil {
		logs.Errorf("head file error: %v", err)
		return false
	}
	if resp.StatusCode == http.StatusOK {
		return true
	}
	return false
}
