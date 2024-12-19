package upload

import (
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/storage"
	"gorm.io/gorm"
)

// UploadCustomerImage UploadCustomerImage
func UploadCustomerImage(ctx *gin.Context, db *gorm.DB, purpose string) {
	var (
		logger    = runtime.Logger(ctx)
		companyID = runtime.CompanyID(ctx)
		uin       = runtime.Uin(ctx)
	)
	f, fh, err := ctx.Request.FormFile("file")
	if err != nil {
		logger.Errorf("upload image error: %v", err)
		runtime.BadRequest(ctx, "参数错误")
		return
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	fi := &storage.FileInfo{
		Purpose:     purpose,
		CompanyID:   companyID,
		Uin:         uin,
		Filename:    fh.Filename,
		Size:        fh.Size,
		FileExt:     ext,
		StoragePath: "/" + storage.GenerateFileStoragePath(purpose, uin, ext),
	}

	st, err := storage.LoadStorager(purpose)
	if err != nil {
		logger.Errorf("upload image error: %v", err)
		runtime.InternalError(ctx, "服务器错误")
		return
	}
	err = st.Save(ctx, fi, f)
	if err != nil {
		logger.Errorf("upload image error: %v", err)
		runtime.InternalError(ctx, "服务器错误")
		return
	}
	fi.PublicURL = st.GetPublicURL(fi.StoragePath, false)

	if err := db.Create(fi).Error; err != nil {
		logger.Errorf("upload image error: %v", err)
		runtime.InternalError(ctx, "服务器错误")
		return
	}

	resp := &UploadImageResponse{
		Response: FileInfo{
			FileID: fi.ID,
			URL:    fi.PublicURL,
		},
	}
	ctx.JSON(200, resp)
}
