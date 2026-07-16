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

type WeedFileChunk struct {
	FileID string `json:"file_id"`
	Size   int64  `json:"size"`
}

type WeedFileEntry struct {
	FullPath string          `json:"FullPath"`
	Mode     fs.FileMode     `json:"Mode"`
	Mime     string          `json:"Mime"`
	Md5      string          `json:"Md5"`
	FileSize int64           `json:"FileSize"`
	Chunks   []WeedFileChunk `json:"chunks"`
}

type WeedFileListResponse struct {
	Path    string           `json:"Path"`
	Entries []*WeedFileEntry `json:"Entries"`
}

type WalkWeedFunc func(ctx context.Context, filer *goseaweedfs.Filer, pathStartWith string, ent *WeedFileEntry, err error) error

func WalkWeedDir(ctx context.Context, filer *goseaweedfs.Filer, rootDir string, wf WalkWeedFunc) error {
	lastFilename := ""
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
