package common

import (
	"errors"
	"strconv"
	"strings"
)

func YuanStringToFen(money string) (int64, error) {
	money = strings.TrimSpace(money)
	if money == "" {
		return 0, errors.New("money 为空")
	}
	if strings.HasPrefix(money, "-") {
		return 0, errors.New("money 不能为负数")
	}

	parts := strings.SplitN(money, ".", 2)
	intPart := parts[0]
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}
	if intPart == "" {
		return 0, errors.New("money 格式错误")
	}
	if len(fracPart) > 2 {
		return 0, errors.New("money 小数位过长")
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}

	yuan, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || yuan < 0 {
		return 0, errors.New("money 整数部分错误")
	}
	frac := int64(0)
	if fracPart != "" {
		frac, err = strconv.ParseInt(fracPart, 10, 64)
		if err != nil || frac < 0 {
			return 0, errors.New("money 小数部分错误")
		}
	}
	return yuan*100 + frac, nil
}

func YuanFloatToFen(amount float64) (int64, error) {
	return YuanStringToFen(strconv.FormatFloat(amount, 'f', 2, 64))
}
