package i18n

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// T 是简化的翻译函数，用于没有参数的简单字符串
func T(lang string, messageID string) string {
	langTag := MatchLanguage(lang)
	localizer := NewLocalizer(langTag)
	if localizer == nil {
		return messageID
	}
	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID: messageID,
	})
	if err != nil {
		return messageID
	}

	return msg
}

// TWithData 用于包含模板数据的翻译
func TWithData(lang string, messageID string, templateData map[string]interface{}) string {
	langTag := MatchLanguage(lang)
	localizer := NewLocalizer(langTag)
	if localizer == nil {
		return messageID
	}
	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})

	if err != nil {
		return messageID
	}

	return msg
}
