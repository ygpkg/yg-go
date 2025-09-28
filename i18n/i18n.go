package i18n

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

var (
	cfg                I18nConfig
	localizers         = make(map[language.Tag]*i18n.Localizer)
	matcher            language.Matcher
	sysDefaultLanguage = language.SimplifiedChinese // 系统默认语言
)

type I18nConfig struct {
	SupportedLanguages []language.Tag `json:"supported_languages"`
	DefaultLanguage    language.Tag   `json:"default_language"`
}

var DefaultConfig = I18nConfig{
	SupportedLanguages: []language.Tag{
		language.SimplifiedChinese, // zh-Hans
	},
	DefaultLanguage: language.SimplifiedChinese, // zh-Hans
}

type LocalesFS interface {
	ReadFile(name string) ([]byte, error)
}

// Init 初始化
func Init(i18nCfg I18nConfig, fs LocalesFS) {
	cfg = i18nCfg

	matcher = language.NewMatcher(cfg.SupportedLanguages)
	defaultLang := matchLanguageFromSupported(cfg.DefaultLanguage.String(), cfg.SupportedLanguages)
	cfg.DefaultLanguage = defaultLang
	// 没有设置语言，或者设置的语言不在支持列表中，使用系统默认语言
	if defaultLang == language.Und {
		cfg.DefaultLanguage = sysDefaultLanguage
	}

	// 初始化 i18n Bundle
	bundle := i18n.NewBundle(cfg.DefaultLanguage)
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)
	for _, lang := range cfg.SupportedLanguages {
		fPath := lang.String() + ".yaml"
		data, err := fs.ReadFile(fPath)
		if err != nil {
			panic(err)
		}
		_, err = bundle.ParseMessageFileBytes(data, fPath)
		if err != nil {
			panic(err)
		}
		loc := i18n.NewLocalizer(bundle, lang.String())
		localizers[lang] = loc
	}
}

// NewLocalizer 创建一个新的本地化器
func NewLocalizer(lang language.Tag) *i18n.Localizer {
	localizer, exists := localizers[lang]
	if !exists {
		localizer = localizers[cfg.DefaultLanguage]
	}
	return localizer
}

// MatchLanguage 检测语言(从语言配置中进行匹配)，如果没有匹配到，返回默认语言
func MatchLanguage(acceptLanguage string) (l language.Tag) {
	l = cfg.DefaultLanguage
	matchLang := matchLanguageFromSupported(acceptLanguage, cfg.SupportedLanguages)
	if matchLang != language.Und {
		l = matchLang
	}
	return
}

// matchLanguageFromSupported 从支持的语言列表中匹配语言
func matchLanguageFromSupported(acceptLanguage string, supported []language.Tag) (l language.Tag) {
	// 没有支持的语言，返回 Und
	if len(supported) == 0 {
		return language.Und
	}

	// 默认返回第一个支持的语言
	l = supported[0]
	if acceptLanguage == "" {
		return
	}
	userPrefs, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err != nil {
		return
	}

	_, index, confidence := language.NewMatcher(supported).Match(userPrefs...)
	if confidence == language.No {
		return
	}
	return supported[index]
}
