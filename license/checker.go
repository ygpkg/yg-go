package license

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/types"
	"gorm.io/gorm"
)

const (
	MillSecondsTemplate = "2006-01-02 15:04:05.999"
)

// Checker 负责执行完整的校验和日志记录流程
type Checker struct {
	db      *gorm.DB
	env     Environment
	hmacKey [32]byte
	preHash string
}

func NewChecker(db *gorm.DB, env Environment) *Checker {
	return &Checker{db: db, env: env}
}

// PerformCheck 是唯一的公开方法，协调整个校验和日志记录
func (c *Checker) PerformCheck(ctx context.Context) {
	var status ValidationStatus
	var message string

	// 运行核心检查逻辑，它会返回最终状态和消息
	status, message = c.runCoreChecks(ctx)

	// 无论检查结果如何，都记录到数据库
	if err := c._logResult(ctx, status, message); err != nil {
		logs.ErrorContextf(ctx, "CRITICAL: Failed to log license check result: %v", err)
	}
}

func (c *Checker) runCoreChecks(ctx context.Context) (ValidationStatus, string) {
	// 1. 从环境中获取所有必要信息
	uid, err := c.env.GetUID(ctx)
	if err != nil {
		return StatusEnvError, fmt.Sprintf("Failed to get environment UID: %v", err)
	}
	rawLicense, err := c.env.GetRawLicense(ctx)
	if err != nil {
		return StatusEnvError, fmt.Sprintf("Failed to get raw license: %v", err)
	}
	publicKey, err := c.env.GetPublicKey(ctx)
	if err != nil {
		return StatusEnvError, fmt.Sprintf("Failed to get public key: %v", err)
	}

	// 2. 校验签名并解析元数据
	meta, err := c._verifySignatureAndParse(ctx, rawLicense, publicKey)
	if err != nil {
		return StatusInvalidSignature, err.Error()
	}

	// 3. 校验元数据
	if err := c._verifyMetadata(ctx, meta, uid); err != nil {
		status := StatusInternalError
		if errors.Is(err, ErrLicenseUIDNotMatch) {
			status = StatusUIDMismatch
		} else if errors.Is(err, ErrLicenseExpired) {
			status = StatusExpired
		}
		return status, err.Error()
	}

	// 4. 校验哈希链
	c.hmacKey = sha256.Sum256([]byte(meta.UID + meta.Seed))
	if err := c._verifyHashChain(ctx); err != nil {
		return StatusTampered, err.Error()
	}

	// 所有检查通过
	return StatusValid, StatusValid.String()
}

func (c *Checker) _verifySignatureAndParse(ctx context.Context, rawLicense string, pubKey *rsa.PublicKey) (*Meta, error) {
	parts := strings.Split(rawLicense, ".")
	if len(parts) != 2 {
		logs.ErrorContextf(ctx, "Invalid license format: %s", rawLicense)
		return nil, fmt.Errorf("invalid license format with %d parts", len(parts))
	}
	signature, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		logs.ErrorContextf(ctx, "invalid base64 for signature: %v", err)
		return nil, fmt.Errorf("invalid base64 for signature: %w", err)
	}
	jsonData, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		logs.ErrorContextf(ctx, "invalid base64 for data: %v", err)
		return nil, fmt.Errorf("invalid base64 for data: %w", err)
	}

	hashed := sha256.Sum256(jsonData)
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], signature); err != nil {
		logs.ErrorContextf(ctx, "invalid signature: %v", err)
		return nil, ErrLicenseSignatureWrong
	}

	var meta Meta
	if err := json.Unmarshal(jsonData, &meta); err != nil {
		logs.ErrorContextf(ctx, "failed to unmarshal license metadata: %v", err)
		return nil, fmt.Errorf("failed to unmarshal license meta: %w", err)
	}
	return &meta, nil
}

func (c *Checker) _verifyMetadata(ctx context.Context, meta *Meta, currentUID string) error {
	if meta.UID != currentUID {
		logs.ErrorContextf(ctx, "metadata UID does not match,license_UID[%v],currentUID[%v]", meta.UID, currentUID)
		return ErrLicenseUIDNotMatch
	}
	if time.Now().After(meta.ExpiredAt) {
		logs.ErrorContextf(ctx, "metadata expired at[%v] now[%v]", meta.ExpiredAt, time.Now())
		return ErrLicenseExpired
	}
	return nil
}

func (c *Checker) _verifyHashChain(ctx context.Context) error {
	var dailyLogs []DailyLog
	if err := c.db.WithContext(ctx).Order("date asc").Find(&dailyLogs).Error; err != nil {
		logs.ErrorContextf(ctx, "failed to find daily logs: %v", err)
		return fmt.Errorf("failed to query daily logs: %w", err)
	}

	if len(dailyLogs) == 0 {
		logs.InfoContextf(ctx, "daily logs is empty,valid hashchain passed")
		mac := hmac.New(sha256.New, c.hmacKey[:])
		mac.Write(c.hmacKey[:])
		c.preHash = hex.EncodeToString(mac.Sum(nil))
		return nil
	}

	// 检查时间是否回拨
	lastLog := dailyLogs[len(dailyLogs)-1]
	if time.Now().Before(lastLog.Date) {
		logs.ErrorContextf(ctx, "system time appears to have been moved backwards, last log date: %v", lastLog.Date)
		return fmt.Errorf("system time appears to have been moved backwards, last log date: %v", lastLog.Date)
	}

	// 校验哈希链
	c.preHash = hex.EncodeToString(c.hmacKey[:])
	for i, logEntry := range dailyLogs {
		if i == 0 {
			mac := hmac.New(sha256.New, c.hmacKey[:])
			mac.Write(c.hmacKey[:])
			c.preHash = hex.EncodeToString(mac.Sum(nil))
		}
		mac := hmac.New(sha256.New, c.hmacKey[:])
		d := logEntry.Date.Format(MillSecondsTemplate)
		mac.Write([]byte(d + c.preHash))
		expectedHash := hex.EncodeToString(mac.Sum(nil))

		if expectedHash != logEntry.CurrentHash {
			logs.ErrorContextf(ctx, "hash chain integrity compromised at date: %s", logEntry.Date)
			return ErrLicenseTampered
		}
		c.preHash = logEntry.CurrentHash
	}

	return nil
}

func (c *Checker) _logResult(ctx context.Context, status ValidationStatus, message string) error {
	mac := hmac.New(sha256.New, c.hmacKey[:])
	now := time.Now()
	d := now.Format(MillSecondsTemplate)
	mac.Write([]byte(d + c.preHash))
	currHash := hex.EncodeToString(mac.Sum(nil))

	var validFlag types.Bool = 1
	if status != StatusValid {
		validFlag = -1
	}

	logEntry := DailyLog{
		Date:         now,
		PreviousHash: c.preHash,
		CurrentHash:  currHash,
		Valid:        validFlag,
		Message:      message,
	}

	return c.db.WithContext(ctx).Create(&logEntry).Error
}
