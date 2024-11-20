package storage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/httptools"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/random"
	"github.com/ygpkg/yg-go/settings"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	defaultPartSize int64 = 1024 * 1024 * 5 // 5MB
)

type iUploader interface {
	Storager

	InitUploadTask(ctx context.Context, tempFile *TempFile) error
	ListUploadExistsTrunk(ctx context.Context, tempFile *TempFile) ([]int, error)
	UploadTrunk(ctx context.Context, tempFile *TempFile, partNumber int, r io.Reader, size int64) error
	CompleteUploadTask(ctx context.Context, tempFile *TempFile) error
}

// Uploader 大文件分片上传
type Uploader struct {
	ctx    context.Context
	cfg    *config.StorageConfig
	logger *zap.SugaredLogger

	tempFile *TempFile
	updr     iUploader
}

func getUploadProvider(group, key string) (*config.StorageConfig, iUploader, error) {
	var cfg config.StorageConfig
	err := settings.GetYaml(group, key, &cfg)
	if err != nil {
		logs.Errorf("get uploader storage config error: %v", err)
		return nil, nil, err
	}
	if cfg.Tencent == nil {
		logs.Errorf("get uploader storage config error: tenant config is nil")
		return &cfg, nil, fmt.Errorf("storage config error")
	}
	tc, err := NewTencentCos(*cfg.Tencent, cfg.StorageOption)
	if err != nil {
		logs.Errorf("new tencent cos error: %v", err)
		return &cfg, nil, err
	}
	return &cfg, tc, nil
}

// GetUploader 获取大文件上传器
func GetUploader(ctx context.Context, logger *zap.SugaredLogger, group, key string, req InitMultipartUploadRequest) (*Uploader, error) {
	cfg, tc, err := getUploadProvider(group, key)
	if err != nil {
		return nil, err
	}
	var tempFile *TempFile
	if req.UploadID != "" {
		tempFile, err = GetTempFileByThirdUploadID(req.CompanyID, req.UploadID)
		if err != nil {
			logger.Errorf("get temp file error: %v", err)
			return nil, err
		}
	} else {
		tempFile, err = GetTempFileByHash(req.SHA1, req.Size)
		if err == gorm.ErrRecordNotFound {
			return newUploader(ctx, logger, cfg, tc, req)
		} else if err != nil {
			logger.Errorf("get temp file error: %v", err)
			return nil, err
		}
	}

	up := &Uploader{
		cfg:      cfg,
		logger:   logger,
		ctx:      ctx,
		tempFile: tempFile,
		updr:     tc,
	}

	return up, nil
}

// newUploader 创建大文件上传器
func newUploader(ctx context.Context, logger *zap.SugaredLogger,
	cfg *config.StorageConfig, updr iUploader, req InitMultipartUploadRequest) (*Uploader, error) {
	tempFile := &TempFile{
		CompanyID:  req.CompanyID,
		CustomerID: req.CustomerID,
		Purpose:    cfg.Purpose,
		Filename:   req.Filename,
		FileExt:    strings.ToLower(filepath.Ext(req.Filename)),
		ChunkHash:  req.SHA1,
		Size:       req.Size,
		ExpiredAt:  time.Now().Add(time.Hour * 24 * 7),
		PartSize:   defaultPartSize,
		PartCount:  GetPartCount(req.Size, defaultPartSize),
	}
	tempFile.MIMEType = httptools.TransformExt2ContentType(tempFile.FileExt)
	tempFile.StoragePath = fmt.Sprintf("/%d/%s/%s-%d-%s%s",
		tempFile.CompanyID, tempFile.Purpose, time.Now().Format("20060102"), tempFile.CustomerID,
		random.String(7), tempFile.FileExt)

	if err := updr.InitUploadTask(ctx, tempFile); err != nil {
		logger.Errorf("init upload task error: %v", err)
		return nil, err
	}

	if err := SaveTempFile(tempFile); err != nil {
		logger.Errorf("save temp file error: %v", err)
		return nil, err
	}

	up := &Uploader{
		cfg:      cfg,
		ctx:      ctx,
		logger:   logger,
		tempFile: tempFile,
		updr:     updr,
	}

	return up, nil
}

// ListTrunks 列出分片
func (u *Uploader) ListTrunks() (*CheckMultipartUploadResponse, error) {
	resp := &CheckMultipartUploadResponse{
		UploadID:  u.tempFile.ThirdUploadID,
		PartCount: int(u.tempFile.PartCount),
		PartSize:  u.tempFile.PartSize,
	}
	exiPartNumbers, err := u.updr.ListUploadExistsTrunk(u.ctx, u.tempFile)
	if err != nil {
		logs.Errorf("list upload trunk error: %v", err)
		return nil, err
	}
	resp.PartNumbersUploaded = exiPartNumbers
	resp.PartNumbersNeedUpload = []int{}
	if len(exiPartNumbers) == int(u.tempFile.PartCount) {
		resp.UploadStatus = FileStatusUploadWaitComp
		return resp, nil
	}
	resp.UploadStatus = FileStatusUploading
	resp.PartNumbersNeedUpload = getNeedUploadPartNumbers(u.tempFile.PartCount, exiPartNumbers)

	return resp, nil
}

// UploadPart 上传分片
func (u *Uploader) UploadPart(partNumber int, data io.Reader, size int64) error {
	err := u.updr.UploadTrunk(u.ctx, u.tempFile, partNumber, data, size)
	if err != nil {
		u.logger.Errorf("upload trunk error: %v", err)
		return err
	}
	return nil
}

// CompleteUpload 合并分片
func (u *Uploader) CompleteUpload() (*FileInfo, error) {
	err := u.updr.CompleteUploadTask(u.ctx, u.tempFile)
	if err != nil {
		u.logger.Errorf("complete upload error: %v", err)
		return nil, err
	}

	fi := &FileInfo{
		CompanyID:   u.tempFile.CompanyID,
		CustomerID:  u.tempFile.CustomerID,
		EmployeeID:  u.tempFile.EmployeeID,
		Purpose:     u.tempFile.Purpose,
		Filename:    u.tempFile.Filename,
		FileExt:     u.tempFile.FileExt,
		MIMEType:    u.tempFile.MIMEType,
		ChunkHash:   u.tempFile.ChunkHash,
		Size:        u.tempFile.Size,
		StoragePath: u.tempFile.StoragePath,
		CopyNumber:  1,
		Status:      FileStatusNormal,
	}
	fi.PublicURL = u.updr.GetPublicURL(fi.StoragePath, false)
	err = dbtools.Core().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(fi).Error; err != nil {
			u.logger.Errorf("create file info error: %v", err)
			return err
		}
		if err := tx.Delete(u.tempFile).Error; err != nil {
			u.logger.Errorf("delete temp file error: %v", err)
			return err
		}
		return nil
	})
	if err != nil {
		u.logger.Errorf("create file info error: %v", err)
		return nil, err
	}
	return fi, nil
}

// Cancel 取消上传
func (u *Uploader) Cancel() error {
	return nil
}

// GetPartCount 获取分片数量
func GetPartCount(size int64, partSize int64) int64 {
	if size%partSize == 0 {
		return size / partSize
	}
	return size/partSize + 1
}

// getNeedUploadPartNumbers 获取需要上传的分片
func getNeedUploadPartNumbers(partCount int64, exiPartNumbers []int) []int {
	var needUploadPartNumbers []int
	for i := 0; i < int(partCount); i++ {
		if !contains(exiPartNumbers, i) {
			needUploadPartNumbers = append(needUploadPartNumbers, i)
		}
	}
	return needUploadPartNumbers
}

// contains 判断是否包含
func contains(arr []int, val int) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}
