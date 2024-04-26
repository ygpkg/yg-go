package email

import (
	"fmt"
	"net"
	"strconv"

	"github.com/ygpkg/yg-go/settings"
	"github.com/ygpkg/yg-go/utils/logs"
	gomail "gopkg.in/gomail.v2"
)

const (
	smtpSettingGroup = "smtp"
)

// SMTPOption 发邮件参数
type SMTPOption struct {
	Addr     string `yaml:"addr"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Nickname string `yaml:"nickname"`
}

// Validity 检查合法性
func (opt SMTPOption) Validity() error {
	if opt.Addr == "" {
		return fmt.Errorf("required smtp addr")
	}
	if opt.Username == "" {
		return fmt.Errorf("required smtp username")
	}
	if opt.Password == "" {
		return fmt.Errorf("required smtp password")
	}
	_, _, err := net.SplitHostPort(opt.Addr)
	if err != nil {
		return fmt.Errorf("invalid smtp addr: %s", opt.Addr)
	}
	return nil
}

// SettingGroupKeys .
func (opt SMTPOption) SettingGroupKeys() (string, []string) {
	return smtpSettingGroup, []string{"addr", "username", "password", "nickname"}
}

// ToSettings 转换为数据库配置项
func (opt SMTPOption) ToSettings() []*settings.SettingItem {
	sets := []*settings.SettingItem{
		{
			Name:      "SMTP地址",
			Describe:  "邮箱的SMTP地址",
			Group:     smtpSettingGroup,
			Key:       "addr",
			ValueType: settings.ValueText,
			Value:     opt.Addr,
		}, {
			Name:      "账户名称",
			Describe:  "邮箱的SMTP账号名称",
			Group:     smtpSettingGroup,
			Key:       "username",
			ValueType: settings.ValueText,
			Value:     opt.Username,
		}, {
			Name:      "账号密码",
			Describe:  "邮箱的SMTP账号密码",
			Group:     smtpSettingGroup,
			Key:       "password",
			ValueType: settings.ValuePassword,
			Value:     opt.Password,
		}, {
			Name:      "发送人昵称",
			Describe:  "邮箱的发送人昵称",
			Group:     smtpSettingGroup,
			Key:       "nickname",
			ValueType: settings.ValueText,
			Value:     opt.Nickname,
		},
	}
	return sets
}

// FromSettings 从数据库加载
func (opt *SMTPOption) FromSettings(sets []*settings.SettingItem) error {
	for _, set := range sets {
		switch set.Key {
		case "username":
			opt.Username = set.Value
		case "addr":
			opt.Addr = set.Value
		case "password":
			opt.Password = set.Value
		case "nickname":
			opt.Nickname = set.Value
		}
	}
	return opt.Validity()
}

// EmailAccount smtp client
type EmailAccount struct {
	SMTPOption

	cli *gomail.Dialer
}

// NewAccount new smtp client
func NewAccount(opt SMTPOption) (*EmailAccount, error) {
	host, sport, err := net.SplitHostPort(opt.Addr)
	if err != nil {
		logs.Errorf("[email] new client failed, %s", err)
		return nil, err
	}

	port, err := strconv.Atoi(sport)
	if err != nil {
		logs.Errorf("[email] new email client failed, %s", err)
		return nil, err
	}

	smtpCli := &EmailAccount{
		SMTPOption: opt,
		cli:        gomail.NewDialer(host, port, opt.Username, opt.Password),
	}

	return smtpCli, nil
}

// SendHTML 发送HTML内容
func (a *EmailAccount) SendHTML(title, body string, sendTo ...string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", fmt.Sprintf("%s<%s>", a.Nickname, a.Username))
	m.SetHeaders(map[string][]string{"To": sendTo})
	m.SetHeader("Subject", title)
	m.SetBody("text/html; charset=UTF-8", body)
	err := a.cli.DialAndSend(m)
	return err
}
