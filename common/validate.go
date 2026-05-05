package common

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var Validate *validator.Validate

func init() {
	Validate = validator.New()
}

// ValidationErrorMessage converts validator errors into user-friendly messages.
// If err is not a validator.ValidationErrors, it falls back to err.Error().
func ValidationErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return err.Error()
	}

	messages := make([]string, 0, len(validationErrors))
	seen := make(map[string]struct{}, len(validationErrors))
	for _, fieldError := range validationErrors {
		message := formatValidationFieldError(fieldError)
		if message == "" {
			continue
		}
		if _, ok := seen[message]; ok {
			continue
		}
		seen[message] = struct{}{}
		messages = append(messages, message)
	}

	if len(messages) == 0 {
		return err.Error()
	}

	return strings.Join(messages, "；")
}

func formatValidationFieldError(fieldError validator.FieldError) string {
	fieldLabel := validationFieldLabel(fieldError.Field())

	switch fieldError.Tag() {
	case "required":
		return fmt.Sprintf("%s不能为空", fieldLabel)
	case "max":
		if fieldError.Kind() == reflect.String {
			return fmt.Sprintf("%s长度不能超过 %s 位", fieldLabel, fieldError.Param())
		}
		return fmt.Sprintf("%s不能大于 %s", fieldLabel, fieldError.Param())
	case "min":
		if fieldError.Kind() == reflect.String {
			return fmt.Sprintf("%s长度不能小于 %s 位", fieldLabel, fieldError.Param())
		}
		return fmt.Sprintf("%s不能小于 %s", fieldLabel, fieldError.Param())
	case "gt":
		return fmt.Sprintf("%s必须大于 %s", fieldLabel, fieldError.Param())
	case "gte":
		return fmt.Sprintf("%s必须大于等于 %s", fieldLabel, fieldError.Param())
	case "lt":
		return fmt.Sprintf("%s必须小于 %s", fieldLabel, fieldError.Param())
	case "lte":
		return fmt.Sprintf("%s必须小于等于 %s", fieldLabel, fieldError.Param())
	case "email":
		return fmt.Sprintf("%s格式不正确", fieldLabel)
	default:
		return fmt.Sprintf("%s校验失败(%s)", fieldLabel, fieldError.Tag())
	}
}

func validationFieldLabel(fieldName string) string {
	switch fieldName {
	case "Username":
		return "用户名"
	case "Password":
		return "密码"
	case "DisplayName":
		return "显示名称"
	case "Email":
		return "邮箱"
	case "BaseMultiplier":
		return "基础倍率"
	default:
		return fieldName
	}
}
