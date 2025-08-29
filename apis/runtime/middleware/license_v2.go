package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/apiobj"
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/license"
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

func LicenseCheckV2(ctx *gin.Context) {
	if err := LogEntryCheck(ctx); err != nil {
		logs.ErrorContextf(ctx, "license check failed: %v", err)
		ctx.AbortWithStatusJSON(http.StatusForbidden, apiobj.BaseResponse{Code: 403, Message: "License认证失败"})
		return
	}
	logs.DebugContextf(ctx, "license check succeed")
}

// LogEntryCheck will check latest log entry
func LogEntryCheck(ctx context.Context) error {
	lg := &licensetool.DailyLog{}
	if err := dbtools.Core().WithContext(ctx).Unscoped().Order("date desc").First(&lg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logs.InfoContextf(ctx, "License check log entry not found")
			return err
		} else {
			logs.ErrorContextf(ctx, "Failed to get latest log entry: %v", err)
			return err
		}
	}

	//if log entry log in today and found license already outdated, then no need to create new log entry
	//otherwise verify license and create new log entry
	if lg.Valid != 1 {
		logs.ErrorContextf(ctx, "License check log entry[%+v] invalid, message: %v", *lg, lg.Message)
		return licensetool.ErrInvalidLogEntry
	}
	return nil
}
