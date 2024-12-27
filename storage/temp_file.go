package storage

import (
	"time"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/dbtools"
	"gorm.io/gorm"
)

// TempFile 临时文件信息
type TempFile struct {
	gorm.Model

	CompanyID  uint `gorm:"column:company_id;index" json:"company_id"`
	Uin        uint `gorm:"column:uin;index" json:"uin"`
	EmployeeID uint `gorm:"column:employee_id" json:"employee_id"`
	// Purpose is the purpose of the file.
	Purpose config.FilePurpose `gorm:"column:purpose;type:varchar(32)" json:"purpose"`

	Filename string `gorm:"column:filename;type:varchar(128)" json:"filename"`
	// FileExt is the extension of the file.
	FileExt  string `gorm:"column:file_ext;type:varchar(8)" json:"file_ext"`
	MIMEType string `gorm:"column:mime_type" json:"mime_type"`
	// Size is the size of the file.
	Size int64 `gorm:"column:size" json:"size"`
	// ChunkHash
	ChunkHash string `gorm:"column:chunk_hash;type:varchar(64)" json:"chunk_hash"`
	// StoragePath is the path of the file in the storage.
	StoragePath string `gorm:"column:path;type:varchar(128)" json:"-"`
	// PublicURL is the public URL of the file.
	PublicURL string `gorm:"column:public_url;type:varchar(256)" json:"-"`

	// ErrMsg is the error message of the file.
	ErrMsg string `gorm:"column:err_msg;type:varchar(256)" json:"err_msg"`

	// Annotations is the annotations of the file.
	Annotations    map[string]string `gorm:"-" json:"annotations"`
	AnnotationsStr string            `gorm:"column:annotations;type:varchar(255)" json:"-"`

	// ThirdUploadID 对象存储的分片上传ID
	ThirdUploadID string `gorm:"column:third_upload_id;type:varchar(128)" json:"-"`
	// ExpiredAt 对象存储的分片上传ID过期时间
	ExpiredAt time.Time `gorm:"column:expired_at" json:"-"`

	// PartSize 对象存储的分片大小
	PartSize int64 `gorm:"column:part_size" json:"-"`
	// PartCount 对象存储的分片数量
	PartCount int64 `gorm:"column:part_count" json:"-"`
}

// TableName 表名
func (*TempFile) TableName() string { return TableNameTempFile }

// GetTempFileByHash 根据hash获取临时文件
func GetTempFileByHash(hashstr string, size int64) (*TempFile, error) {
	var tempFile TempFile
	sql := dbtools.Core().Model(tempFile).
		Where("size = ?", size).
		Where("chunk_hash = ?", hashstr)

	err := sql.First(&tempFile).Error
	if err != nil {
		return nil, err
	}
	if tempFile.ID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &tempFile, nil
}

// GetTempFileByThirdUploadID 根据thirdUploadID获取临时文件
func GetTempFileByThirdUploadID(companyID uint, thirdUploadID string) (*TempFile, error) {
	var tempFile TempFile
	err := dbtools.Core().Model(tempFile).
		Where("company_id = ? AND third_upload_id = ?", companyID, thirdUploadID).
		Last(&tempFile).Error
	if err != nil {
		return nil, err
	}
	if tempFile.ID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &tempFile, nil
}

// SaveTempFile 保存临时文件
func SaveTempFile(tempFile *TempFile) error {
	err := dbtools.Core().Create(tempFile).Error
	if err != nil {
		return err
	}
	return nil
}
