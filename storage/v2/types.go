package v2

import (
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/random"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	TableNameFileInfo = "core_upload_files"
	TableNameTempFile = "core_upload_files_tmp"
)

type FileStatus string

const (
	FileStatusUnknown        FileStatus = "unknown"
	FileStatusInit           FileStatus = "init"
	FileStatusUploading      FileStatus = "uploading"
	FileStatusNormal         FileStatus = "normal"
	FileStatusDeleted        FileStatus = "deleted"
	FileStatusFailed         FileStatus = "failed"
	FileStatusAborted        FileStatus = "aborted"
	FileStatusUploadWaitComp FileStatus = "upload_wait_comp"
	FileStatusUploadSuccess  FileStatus = "upload_success"
)

type FileInfo struct {
	gorm.Model

	CompanyID        uint             `gorm:"column:company_id" json:"company_id"`
	Uin              uint             `gorm:"column:uin" json:"uin"`
	Purpose          config.FilePurpose `gorm:"column:purpose;type:varchar(32)" json:"purpose"`
	Filename         string           `gorm:"column:filename;type:varchar(128)" json:"filename"`
	FileExt          string           `gorm:"column:file_ext;type:varchar(8)" json:"file_ext"`
	MIMEType         string           `gorm:"column:mime_type" json:"mime_type"`
	Size             int64            `gorm:"column:size" json:"size"`
	Hash             string           `gorm:"column:hash;type:varchar(256);index" json:"hash"`
	ChunkHash        string           `gorm:"column:chunk_hash;type:varchar(64)" json:"chunk_hash"`
	StoragePath      string           `gorm:"column:path;type:varchar(128)" json:"-"`
	PublicURL        string           `gorm:"column:public_url;type:varchar(256);index" json:"-"`
	CopyNumber       int              `gorm:"column:copy_number;type:int;default:1" json:"-"`
	UploadChunkSize  int64            `gorm:"column:upload_chunk_size;type:int;comment:分片大小（字节）"`
	UploadChunkTotal int              `gorm:"column:upload_chunk_total;type:int;comment:分片总数"`
	Status           FileStatus       `gorm:"column:status;type:varchar(32);not null;default:'normal';comment:文件状态"`
	UploadedChunks   []UploadedChunk  `gorm:"column:uploaded_chunks;type:json;serializer:json;comment:已上传分片列表"`
	Progress         float64          `gorm:"column:progress;type:decimal(5,2);not null;default:0.00;comment:上传进度（%）"`
	Exists           bool             `gorm:"column:exists;type:boolean;not null;default:false;comment:是否命中秒传"`
	UploadS3ID       string           `gorm:"column:upload_s3_id;type:varchar(128);default:'';comment:S3 MultipartUpload ID"`
	RenewCount       int              `gorm:"column:renew_count;type:int;not null;default:0;comment:预签名 URL 续签次数"`
	AbortAt          *time.Time       `gorm:"column:abort_at;type:datetime;comment:用户取消上传时间"`
	CompletedAt      *time.Time       `gorm:"column:completed_at;type:datetime;comment:文件上传完成时间"`
	Extra            datatypes.JSON   `gorm:"column:extra;type:json;comment:通用扩展属性"`
}

func (*FileInfo) TableName() string { return TableNameFileInfo }

type UploadedChunk struct {
	PartNumber int    `json:"partNumber"`
	Etag       string `json:"etag"`
}

type TempFile struct {
	gorm.Model

	CompanyID     uint               `gorm:"column:company_id;index" json:"company_id"`
	Uin           uint               `gorm:"column:uin;index" json:"uin"`
	EmployeeID    uint               `gorm:"column:employee_id" json:"employee_id"`
	Purpose       config.FilePurpose `gorm:"column:purpose;type:varchar(32)" json:"purpose"`
	Filename      string             `gorm:"column:filename;type:varchar(128)" json:"filename"`
	FileExt       string             `gorm:"column:file_ext;type:varchar(8)" json:"file_ext"`
	MIMEType      string             `gorm:"column:mime_type" json:"mime_type"`
	Size          int64              `gorm:"column:size" json:"size"`
	ChunkHash     string             `gorm:"column:chunk_hash;type:varchar(64)" json:"chunk_hash"`
	StoragePath   string             `gorm:"column:path;type:varchar(128)" json:"-"`
	PublicURL     string             `gorm:"column:public_url;type:varchar(256)" json:"-"`
	ErrMsg        string             `gorm:"column:err_msg;type:varchar(256)" json:"err_msg"`
	ThirdUploadID string             `gorm:"column:third_upload_id;type:varchar(128)" json:"-"`
	ExpiredAt     time.Time          `gorm:"column:expired_at" json:"-"`
	PartSize      int64              `gorm:"column:part_size" json:"-"`
	PartCount     int64              `gorm:"column:part_count" json:"-"`
}

func (*TempFile) TableName() string { return TableNameTempFile }

type CreateMultipartUploadInput struct {
	Bucket      *string
	StoragePath *string
	ContentType *string
}

type GeneratePresignedURLInput struct {
	Method        *string
	Bucket        *string
	StoragePath   *string
	UploadID      *string
	PartNumber    *int
	ContentType   *string
	ContentLength *int64
	ContentMD5    *string
}

type UploadPartInput struct {
	Bucket      *string
	StoragePath *string
	UploadID    *string
	PartNumber  *int
	Data        io.Reader
}

type CompleteMultipartUploadInput struct {
	Bucket      *string
	StoragePath *string
	UploadID    *string
	Parts       *types.CompletedMultipartUpload
}

type AbortMultipartUploadInput struct {
	Bucket      *string
	StoragePath *string
	UploadID    *string
}

func GenerateFileStoragePath(purpose string, owner uint, fileExt string) string {
	return fmt.Sprintf("%s/%s/%d-%s%s",
		purpose, time.Now().Format("20060102"), owner, random.String(9), fileExt)
}

func GenerateFileStoragePathWithName(purpose string, owner uint, fileName string) string {
	return fmt.Sprintf("%s/%s/%d-%s/%s",
		purpose, time.Now().Format("20060102"), owner, random.String(9), fileName)
}

func GetFileByChunkHash(db *gorm.DB, hashstr string, size int64) (*FileInfo, error) {
	fi := &FileInfo{}
	err := db.Model(fi).
		Where("size = ?", size).
		Where("chunk_hash = ?", hashstr).
		First(fi).Error
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

func GetFileByHash(db *gorm.DB, hashstr string) (*FileInfo, error) {
	fi := &FileInfo{}
	err := db.Model(fi).
		Where("hash = ?", hashstr).
		First(fi).Error
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

func GetFileByPublicURL(db *gorm.DB, publicURL string) (*FileInfo, error) {
	fi := &FileInfo{}
	err := db.Model(fi).
		Where("public_url = ?", publicURL).
		First(fi).Error
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

func GetFileByID(db *gorm.DB, id uint) (*FileInfo, error) {
	if id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	fi := &FileInfo{}
	err := db.Model(fi).Where("id = ?", id).First(fi).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			logs.Warnf("get file by id failed, id = %v, error: %v", id, err)
		}
		return nil, err
	}
	return fi, nil
}

type InitMultipartUploadRequest struct {
	SHA1     string `json:"sha1"`
	Size     int64  `json:"size"`
	Filename string `json:"filename"`

	CompanyID  uint   `json:"-"`
	Uin        uint   `json:"-"`
	EmployeeID uint   `json:"-"`
	UploadID   string `json:"-"`
}

type CheckMultipartUploadResponse struct {
	UploadID              string     `json:"upload_id"`
	PartSize              int64      `json:"part_size"`
	PartCount             int        `json:"part_count"`
	PartNumbersUploaded   []int      `json:"uploaded"`
	PartNumbersNeedUpload []int      `json:"need_upload"`
	UploadStatus          FileStatus `json:"upload_status"`
	FileInfo              *FileInfo  `json:"file_info"`
}
