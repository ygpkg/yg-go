package storage

import (
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/random"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// FileStatus 文件状态
type FileStatus string

const (
	// FileStatusUnknown 未知
	FileStatusUnknown FileStatus = "unknown"
	// FileStatusInit 初始化
	FileStatusInit FileStatus = "init"
	// FileStatusUploading 上传中
	FileStatusUploading FileStatus = "uploading"
	// FileStatusNormal 正常，完成
	FileStatusNormal FileStatus = "normal"
	// FileStatusDeleted 已删除
	FileStatusDeleted FileStatus = "deleted"
	// FileStatusFailed 上传失败
	FileStatusFailed FileStatus = "failed"
	// FileStatusAborted 取消上传
	FileStatusAborted FileStatus = "aborted"

	// FileStatusUploadWaitComp 上传等待完成
	FileStatusUploadWaitComp FileStatus = "upload_wait_comp"
	// FileStatusUploadSuccess 上传成功
	FileStatusUploadSuccess FileStatus = "upload_success"
)

// FileInfo .
type FileInfo struct {
	gorm.Model

	// CompanyID 公司ID
	CompanyID uint `gorm:"column:company_id" json:"company_id"`
	// Uin 用户ID
	Uin uint `gorm:"column:uin" json:"uin"`

	// Purpose 用途分类
	Purpose config.FilePurpose `gorm:"column:purpose;type:varchar(32)" json:"purpose"`
	// Filename 原始文件名
	Filename string `gorm:"column:filename;type:varchar(128)" json:"filename"`
	// FileExt 文件扩展名
	FileExt string `gorm:"column:file_ext;type:varchar(8)" json:"file_ext"`
	// MIMEType MIME类型
	MIMEType string `gorm:"column:mime_type" json:"mime_type"`
	// Size 文件大小
	Size int64 `gorm:"column:size" json:"size"`
	// Hash 文件hash hashMethod:hashValue
	Hash string `gorm:"column:hash;type:varchar(256);index" json:"hash"`
	// ChunkHash is the hash of the chunk of the file.
	ChunkHash string `gorm:"column:chunk_hash;type:varchar(64)" json:"chunk_hash"`
	// StoragePath 存储的相对路径
	StoragePath string `gorm:"column:path;type:varchar(128)" json:"-"`
	// PublicURL 公网访问地址，如果为空，则表示只能通过预签名URL访问
	PublicURL string `gorm:"column:public_url;type:varchar(256);index" json:"-"`

	// CopyNumber 文件副本数量
	CopyNumber       int             `gorm:"column:copy_number;type:int;default:1" json:"-"`
	UploadChunkSize  int64           `gorm:"column:upload_chunk_size;type:int;comment:分片大小（字节）"`
	UploadChunkTotal int             `gorm:"column:upload_chunk_total;type:int;comment:分片总数"`
	Status           FileStatus      `gorm:"column:status;type:varchar(32);not null;default:'normal';comment:文件状态，init：初始化，uploading：上传中，normal：已完成，aborted：已取消，failed：上传失败"`
	UploadedChunks   []UploadedChunk `gorm:"column:uploaded_chunks;type:json;serializer:json;comment:已上传分片列表，例如 [{\"partNumber\":1,\"etag\":\"xxx\"}]"`
	Progress         float64         `gorm:"column:progress;type:decimal(5,2);not null;default:0.00;comment:上传进度（%）"`
	Exists           bool            `gorm:"column:exists;type:boolean;not null;default:false;comment:是否命中秒传"`
	UploadS3ID       string          `gorm:"column:upload_s3_id;type:varchar(128);default:'';comment:S3 MultipartUpload ID"`
	RenewCount       int             `gorm:"column:renew_count;type:int;not null;default:0;comment:预签名 URL 续签次数"`
	AbortAt          *time.Time      `gorm:"column:abort_at;type:datetime;comment:用户取消上传时间"`
	CompletedAt      *time.Time      `gorm:"column:completed_at;type:datetime;comment:文件上传完成时间"`
	Extra            datatypes.JSON  `gorm:"column:extra;type:json;comment:通用扩展属性，存储自定义元数据，额外业务信息"`
}

// TableName table name
func (*FileInfo) TableName() string { return TableNameFileInfo }

type UploadedChunk struct {
	PartNumber int    `json:"partNumber"`
	Etag       string `json:"etag"`
}

// GetFileByChunkHash 通过hash获取文件信息
func GetFileByChunkHash(db *gorm.DB, hashstr string, size int64) (*FileInfo, error) {
	fi := &FileInfo{}
	sql := db.Model(fi).
		Where("size = ?", size).
		Where("chunk_hash = ?", hashstr)

	err := sql.First(fi).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			logs.Errorf("get file by hash failed, %v, %d error: %v", hashstr, size, err)
		}
		return nil, err
	}
	if fi.ID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return fi, nil
}

// GetFileByHash 通过hash获取文件信息
func GetFileByHash(db *gorm.DB, hashstr string) (*FileInfo, error) {
	fi := &FileInfo{}
	sql := db.Model(fi).
		Where("hash = ?", hashstr)

	err := sql.First(fi).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			logs.Errorf("get file by hash failed, %v error: %v", hashstr, err)
		}
		return nil, err
	}
	if fi.ID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return fi, nil
}

// GetFileByPublicURL 通过publicURL获取文件信息
func GetFileByPublicURL(db *gorm.DB, publicURL string) (*FileInfo, error) {
	fi := &FileInfo{}
	sql := db.Model(fi).
		Where("public_url = ?", publicURL)

	err := sql.First(fi).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			logs.Errorf("get file by public url failed, %v error: %v", publicURL, err)
		}
		return nil, err
	}
	if fi.ID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return fi, nil
}

// SaveCopyFile 保存文件副本
func SaveCopyFile(db *gorm.DB, fi *FileInfo) error {
	tErr := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Create(fi).Error
		if err != nil {
			logs.Errorf("create file info failed, %v", err)
			return err
		}
		err = tx.Table(TableNameFileInfo).
			Where("`hash` = ? AND size = ?", fi.Hash, fi.Size).
			Update("copy_number", fi.CopyNumber).Error
		if err != nil {
			logs.Errorf("update copy number failed, %v", err)
			return err
		}
		return nil
	})
	if tErr != nil {
		logs.Errorf("save copy file failed, %v", tErr)
		return tErr
	}
	return nil
}

// GetCompanyFileByID 通过id获取文件信息
func GetCompanyFileByID(db *gorm.DB, companyID, id uint) (*FileInfo, error) {
	if id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	fi := &FileInfo{}
	sql := db.Model(fi).
		Where("company_id = ?", companyID).
		Where("id = ?", id)
	err := sql.First(fi).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			logs.Errorf("get file by id failed, company_id = %v,id = %v,error: %v", companyID, id, err)
		}
		return nil, err
	}

	return fi, nil
}

// GetFileByID 通过id获取文件信息
func GetFileByID(db *gorm.DB, id uint) (*FileInfo, error) {
	if id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	fi := &FileInfo{}
	sql := db.Model(fi).
		Where("id = ?", id)
	err := sql.First(fi).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			logs.Warnf("get file by id failed, id = %v,error: %v", id, err)
		}
		return nil, err
	}

	return fi, nil
}

// GenerateFileStoragePath 生成文件存储路径
func GenerateFileStoragePath(purpose string, owner uint, fileExt string) string {
	storagePath := fmt.Sprintf("%s/%s/%d-%s%s",
		purpose, time.Now().Format("20060102"), owner, random.String(9), fileExt)
	return storagePath
}

// GenerateFileStoragePathWithName 生成文件存储路径
func GenerateFileStoragePathWithName(purpose string, owner uint, fileName string) string {
	storagePath := fmt.Sprintf("%s/%s/%d-%s/%s",
		purpose, time.Now().Format("20060102"), owner, random.String(9), fileName)
	return storagePath
}
