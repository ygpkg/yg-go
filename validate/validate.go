package validate

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

// type Func func(v interface{}) error

// IsEmail 是否是合法邮箱
func IsEmail(value string) error {
	emailRegexp := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

	if !emailRegexp.MatchString(value) {
		return fmt.Errorf("invalid format email: %s", value)
	}
	return nil
}

// IsPhone 是否是国内合法手机号
func IsPhone(value string) error {
	reg := `^1([38796][0-9]|14[57]|5[^4])\d{8}$`
	rgx := regexp.MustCompile(reg)
	if rgx.MatchString(value) {
		return nil
	}
	return fmt.Errorf("手机号码 %s 格式错误", value)
}

// IsCardNumber 是否是国内合法身份证号
func IsCardNumber(value string) error {
	if len(value) != 18 && len(value) != 15 {
		return fmt.Errorf("身份证 %s 长度错误", value)
	}
	reg := `(^[1-9]\d{5}(18|19|([23]\d))\d{2}((0[1-9])|(10|11|12))(([0-2][1-9])|10|20|30|31)\d{3}[0-9Xx])|([1−9]\d5\d2((0[1−9])|(10|11|12))(([0−2][1−9])|10|20|30|31)\d2[0−9Xx])`
	rgx := regexp.MustCompile(reg)
	if rgx.MatchString(value) {
		return nil
	}
	return fmt.Errorf("身份证 %s 格式错误", value)
}

// IsBankAccountNumber 是否是国内合法银行卡号
func IsBankAccountNumber(value string) error {
	if len(value) != 16 && len(value) != 19 {
		return fmt.Errorf("银行卡号 %s 长度错误", value)
	}
	reg := `^(\d+)$`
	rgx := regexp.MustCompile(reg)
	if rgx.MatchString(value) {
		return nil
	}
	return fmt.Errorf("银行卡号 %s 格式错误", value)
}

// IsLetterNumber 是否是国内合法手机号
func IsLetterNumber(value string) error {
	reg := `^[A-Za-z0-9]+$`
	rgx := regexp.MustCompile(reg)
	if rgx.MatchString(value) {
		return nil
	}
	return fmt.Errorf("内容格式错误 %s", value)
}

// IsUsername 是否为合法用户名
func IsUsername(username string) error {
	// 用户名必须为 1-32 个字符
	if len(username) < 3 || len(username) > 32 {
		return fmt.Errorf("用户名长度必须为 3-32 个字符")
	}

	// 用户名只能包含字母数字和破折号（-）
	validChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"
	for _, ch := range username {
		if !strings.ContainsRune(validChars, ch) {
			return fmt.Errorf("用户名只能包含字母数字和破折号（-）")
		}
	}

	// 用户名不能以破折号（-）开头或结尾
	if username[0] == '-' || username[len(username)-1] == '-' {
		return fmt.Errorf("用户名不能以破折号（-）开头或结尾")
	}

	// 用户名不能包含连续的破折号（--）
	if strings.Contains(username, "--") {
		return fmt.Errorf("用户名不能包含连续的破折号（--）")
	}

	return nil
}

// IsTitle 是否为合法密码
func IsTitle(value string) error {
	if len(value) < 5 {
		return fmt.Errorf("标题长度过短")
	}
	return nil
}

// IsPassword 是否为合法密码
func IsPassword(value string) error {
	if len(value) < 6 {
		return fmt.Errorf("密码长度过短")
	}
	return nil
}

var (
	defaultNearbyTimeRange = time.Hour * 24 * 365 * 8
)

// IsNearbyTime 是否为附近的时间
func IsNearbyTime(t time.Time, intervals ...time.Duration) error {
	var (
		now        = time.Now()
		begin, end time.Time
	)
	if len(intervals) > 0 {
		begin = now.Add(intervals[0] * -1)
	} else {
		begin = now.Add(defaultNearbyTimeRange * -1)
	}
	if len(intervals) > 1 {
		end = now.Add(intervals[1])
	} else {
		end = now.Add(defaultNearbyTimeRange)
	}

	if t.Before(begin) {
		return fmt.Errorf("invalid time %s, is before %s", t, begin)
	}
	if t.After(end) {
		return fmt.Errorf("invalid time %s, is after %s", t, end)
	}
	return nil
}

// IsDocumentNumber 是否是公开公告号
func IsDocumentNumber(value string) bool {
	reg := `^[a-zA-Z]{2}\d+[a-zA-Z]$`
	rgx := regexp.MustCompile(reg)
	if rgx.MatchString(value) {
		return true
	}
	return false
}

// IsApplicationNumber 是否是申请号
func IsApplicationNumber(value string) bool {
	reg := `^[a-zA-Z]{2}\d*\.\d$`
	rgx := regexp.MustCompile(reg)
	if rgx.MatchString(value) {
		return true
	}
	return false
}

func IsValidStruct(data any, translate bool) error {
	err := paramValidate.Struct(data)
	if err != nil {
		if !translate {
			return err
		}
		for _, e := range err.(validator.ValidationErrors) {
			return fmt.Errorf("%s", e.Translate(translator))
		}

	}
	return nil
}
