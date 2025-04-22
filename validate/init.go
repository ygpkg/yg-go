package validate

import (
	"reflect"

	"github.com/go-playground/locales/zh_Hans_CN"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/go-playground/validator/v10/translations/zh"
)

var paramValidate *validator.Validate
var translator ut.Translator

func init() {
	paramValidate = validator.New()
	uni := ut.New(zh_Hans_CN.New())
	translator, _ = uni.GetTranslator("zh_Hans_CN")
	_ = zh.RegisterDefaultTranslations(paramValidate, translator)
	paramValidate.RegisterTagNameFunc(func(field reflect.StructField) string {
		label := field.Tag.Get("label")
		return label
	})
}
