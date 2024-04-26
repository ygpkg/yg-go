package types

import (
	"strings"
	"time"
)

const (
	// AppletTimeFormat 小程序时间格式
	AppletTimeFormat = "2006-01-02T15:04:05.000Z"
)

// 小程序时间
type AppletTime time.Time

// UnmarshalJSON 实现json反序列化接口
func (t *AppletTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	str := strings.Trim(string(data), "\"")
	var err error
	now, err := time.ParseInLocation(AppletTimeFormat, str, time.Local)
	*t = AppletTime(now)
	return err
}

// MarshalJSON 实现json序列化接口
func (t AppletTime) MarshalJSON() ([]byte, error) {
	return []byte(time.Time(t).Format(`"` + AppletTimeFormat + `"`)), nil
}

// String 实现string接口
func (t AppletTime) String() string {
	return time.Time(t).Format(AppletTimeFormat)
}

// Time 转换为time.Time
func (t AppletTime) Time() time.Time {
	return time.Time(t)
}
