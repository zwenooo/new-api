package common

import "time"

// GetTodayDateInt 返回当前日期的整数形式（YYYYMMDD）。
func GetTodayDateInt() int {
	return DateToInt(time.Now())
}

// DateToInt 将时间转换为 YYYYMMDD 形式的整数，统一使用本地时区。
func DateToInt(t time.Time) int {
	year, month, day := t.In(time.Local).Date()
	return year*10000 + int(month)*100 + day
}

// GetStartOfDayUnix 返回当天零点的 Unix 时间戳，便于外部在需要时复用。
func GetStartOfDayUnix(t time.Time) int64 {
	local := t.In(time.Local)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	return start.Unix()
}
