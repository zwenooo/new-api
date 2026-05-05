package model

import (
	"fmt"
	"one-api/common"
	"time"

	"gorm.io/gorm"
)

// OrderRevenueBucketStat holds aggregated revenue for one bucket and one order type.
type OrderRevenueBucketStat struct {
	Bucket    string `json:"bucket"`
	Label     string `json:"label"`
	OrderType string `json:"order_type"` // subscription / topup / payg / pay_request / pay_token
	AmountFen int64  `json:"amount_fen"` // sum in fen
}

// OrderRevenueStats describes one natural day/week/month/year revenue view.
type OrderRevenueStats struct {
	Period     string                    `json:"period"`
	AnchorDate string                    `json:"anchor_date"`
	StartDate  string                    `json:"start_date"`
	EndDate    string                    `json:"end_date"`
	TotalFen   int64                     `json:"total_fen"`
	Items      []*OrderRevenueBucketStat `json:"items"`
}

const (
	orderTypeSubscription = "subscription"
	orderTypeTopup        = "topup"
	orderTypePayg         = "payg"
	orderTypePayRequest   = "pay_request"
	orderTypePayToken     = "pay_token"
)

var orderRevenueTypes = []string{
	orderTypeSubscription,
	orderTypeTopup,
	orderTypePayg,
	orderTypePayRequest,
	orderTypePayToken,
}

type bucketAmountRow struct {
	BucketKey string `gorm:"column:bucket_key"`
	AmountFen int64  `gorm:"column:amount_fen"`
}

type rawBucketAmountRow struct {
	Timestamp int64 `gorm:"column:ts"`
	AmountFen int64 `gorm:"column:amount_fen"`
}

type orderRevenueRange struct {
	Period       string
	AnchorDate   string
	Start        time.Time
	End          time.Time
	StartDate    string
	EndDate      string
	BucketKeys   []string
	BucketLabels map[string]string
}

// GetOrderRevenueStats returns revenue stats for one natural day/week/month/year window.
// orderType: "" = all, or one of subscription/topup/payg/pay_request/pay_token
// period: day / week / month / year
// anchorDate: YYYY-MM-DD; empty means today in the provided timezone
func GetOrderRevenueStats(orderType string, period string, anchorDate string, timeZone string) (*OrderRevenueStats, error) {
	if period == "" {
		period = "day"
	}

	now := time.Now()
	loc, err := resolveOrderRevenueLocation(timeZone, now)
	if err != nil {
		return nil, err
	}
	revenueRange, err := buildOrderRevenueRange(period, anchorDate, loc, now)
	if err != nil {
		return nil, err
	}

	targetTypes := selectedOrderRevenueTypes(orderType)
	amounts := make(map[string]int64, len(targetTypes)*len(revenueRange.BucketKeys))

	if orderType == "" || orderType == orderTypeSubscription {
		timestampExpr := orderPaidTimestampExpr("paid_at", "finished_at")
		err := accumulatePaidBucketRows(
			DB.Table("subscription_orders").
				Where(
					fmt.Sprintf("status = ? AND pay_method <> ? AND %s >= ? AND %s < ?", timestampExpr, timestampExpr),
					SubscriptionOrderStatusSuccess, SubscriptionPayMethodBalance, revenueRange.Start.Unix(), revenueRange.End.Unix(),
				),
			timestampExpr,
			orderTypeSubscription,
			loc,
			revenueRange,
			amounts,
		)
		if err != nil {
			return nil, err
		}
	}

	if orderType == "" || orderType == orderTypeTopup {
		timestampExpr := nonZeroTimestampExpr("complete_time")
		err := accumulateTopUpBucketRows(
			DB.Table("top_ups").
				Where(
					fmt.Sprintf("status = ? AND %s >= ? AND %s < ?", timestampExpr, timestampExpr),
					common.TopUpStatusSuccess, revenueRange.Start.Unix(), revenueRange.End.Unix(),
				),
			timestampExpr,
			loc,
			revenueRange,
			amounts,
		)
		if err != nil {
			return nil, err
		}
	}

	if orderType == "" || orderType == orderTypePayg {
		timestampExpr := orderPaidTimestampExpr("paid_at", "finished_at")
		err := accumulatePaidBucketRows(
			DB.Table("payg_orders").
				Where(
					fmt.Sprintf("status = ? AND pay_method <> ? AND %s >= ? AND %s < ?", timestampExpr, timestampExpr),
					PaygOrderStatusSuccess, PaygPayMethodBalance, revenueRange.Start.Unix(), revenueRange.End.Unix(),
				),
			timestampExpr,
			orderTypePayg,
			loc,
			revenueRange,
			amounts,
		)
		if err != nil {
			return nil, err
		}
	}

	if orderType == "" || orderType == orderTypePayRequest {
		timestampExpr := orderPaidTimestampExpr("paid_at", "finished_at")
		err := accumulatePaidBucketRows(
			DB.Table("pay_request_orders").
				Where(
					fmt.Sprintf("status = ? AND pay_method <> ? AND %s >= ? AND %s < ?", timestampExpr, timestampExpr),
					PayRequestOrderStatusSuccess, PayRequestPayMethodBalance, revenueRange.Start.Unix(), revenueRange.End.Unix(),
				),
			timestampExpr,
			orderTypePayRequest,
			loc,
			revenueRange,
			amounts,
		)
		if err != nil {
			return nil, err
		}
	}

	if orderType == "" || orderType == orderTypePayToken {
		timestampExpr := orderPaidTimestampExpr("paid_at", "finished_at")
		err := accumulatePaidBucketRows(
			DB.Table("pay_token_orders").
				Where(
					fmt.Sprintf("status = ? AND pay_method <> ? AND %s >= ? AND %s < ?", timestampExpr, timestampExpr),
					PayTokenOrderStatusSuccess, PayTokenPayMethodBalance, revenueRange.Start.Unix(), revenueRange.End.Unix(),
				),
			timestampExpr,
			orderTypePayToken,
			loc,
			revenueRange,
			amounts,
		)
		if err != nil {
			return nil, err
		}
	}

	items := make([]*OrderRevenueBucketStat, 0, len(revenueRange.BucketKeys)*len(targetTypes))
	totalFen := int64(0)
	for _, bucketKey := range revenueRange.BucketKeys {
		label := revenueRange.BucketLabels[bucketKey]
		for _, typ := range targetTypes {
			amountFen := amounts[orderRevenueAmountKey(bucketKey, typ)]
			items = append(items, &OrderRevenueBucketStat{
				Bucket:    bucketKey,
				Label:     label,
				OrderType: typ,
				AmountFen: amountFen,
			})
			totalFen += amountFen
		}
	}

	return &OrderRevenueStats{
		Period:     revenueRange.Period,
		AnchorDate: revenueRange.AnchorDate,
		StartDate:  revenueRange.StartDate,
		EndDate:    revenueRange.EndDate,
		TotalFen:   totalFen,
		Items:      items,
	}, nil
}

func resolveOrderRevenueLocation(timeZone string, now time.Time) (*time.Location, error) {
	if tz := timeZone; tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return nil, fmt.Errorf("time_zone 无效: %w", err)
		}
		return loc, nil
	}

	if loc := now.Location(); loc != nil {
		return loc, nil
	}
	return time.Local, nil
}

func selectedOrderRevenueTypes(orderType string) []string {
	if orderType == "" {
		return append([]string(nil), orderRevenueTypes...)
	}
	return []string{orderType}
}

func buildOrderRevenueRange(period string, anchorDate string, loc *time.Location, now time.Time) (*orderRevenueRange, error) {
	anchor, err := parseOrderRevenueAnchorDate(anchorDate, loc, now)
	if err != nil {
		return nil, err
	}

	switch period {
	case "day":
		start := anchor
		end := start.AddDate(0, 0, 1)
		return newOrderRevenueRange(period, anchor, start, end, "15:00"), nil
	case "week":
		weekday := int(anchor.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := anchor.AddDate(0, 0, -(weekday - 1))
		end := start.AddDate(0, 0, 7)
		return newOrderRevenueRange(period, anchor, start, end, "2006-01-02"), nil
	case "month":
		start := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, anchor.Location())
		end := start.AddDate(0, 1, 0)
		return newOrderRevenueRange(period, anchor, start, end, "2006-01-02"), nil
	case "year":
		start := time.Date(anchor.Year(), time.January, 1, 0, 0, 0, 0, anchor.Location())
		end := start.AddDate(1, 0, 0)
		return newOrderRevenueRange(period, anchor, start, end, "2006-01"), nil
	default:
		return nil, fmt.Errorf("period 无效: %s", period)
	}
}

func newOrderRevenueRange(period string, anchor time.Time, start time.Time, end time.Time, bucketFormat string) *orderRevenueRange {
	bucketKeys := make([]string, 0)
	bucketLabels := make(map[string]string)
	seen := make(map[string]struct{})

	switch period {
	case "day":
		for bucketTime := start; bucketTime.Before(end); bucketTime = bucketTime.Add(time.Hour) {
			key := bucketTime.Format(bucketFormat)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			bucketKeys = append(bucketKeys, key)
			bucketLabels[key] = key
		}
	case "week":
		for bucketTime := start; bucketTime.Before(end); bucketTime = bucketTime.AddDate(0, 0, 1) {
			key := bucketTime.Format(bucketFormat)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			bucketKeys = append(bucketKeys, key)
			bucketLabels[key] = bucketTime.Format("01-02")
		}
	case "month":
		for bucketTime := start; bucketTime.Before(end); bucketTime = bucketTime.AddDate(0, 0, 1) {
			key := bucketTime.Format(bucketFormat)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			bucketKeys = append(bucketKeys, key)
			bucketLabels[key] = fmt.Sprintf("%d日", bucketTime.Day())
		}
	case "year":
		for bucketTime := start; bucketTime.Before(end); bucketTime = bucketTime.AddDate(0, 1, 0) {
			key := bucketTime.Format(bucketFormat)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			bucketKeys = append(bucketKeys, key)
			bucketLabels[key] = bucketTime.Format("01月")
		}
	}

	return &orderRevenueRange{
		Period:       period,
		AnchorDate:   anchor.Format("2006-01-02"),
		Start:        start,
		End:          end,
		StartDate:    start.Format("2006-01-02"),
		EndDate:      end.Add(-time.Second).Format("2006-01-02"),
		BucketKeys:   bucketKeys,
		BucketLabels: bucketLabels,
	}
}

func parseOrderRevenueAnchorDate(raw string, loc *time.Location, now time.Time) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}

	if raw == "" {
		localNow := now.In(loc)
		return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc), nil
	}

	parsed, err := time.ParseInLocation("2006-01-02", raw, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("date 无效: %w", err)
	}
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc), nil
}

func accumulatePaidBucketRows(query *gorm.DB, timestampColumn string, orderType string, loc *time.Location, revenueRange *orderRevenueRange, amounts map[string]int64) error {
	rows, err := queryBucketAmountRows(query, timestampColumn, "amount_fen", loc, revenueRange)
	if err != nil {
		return err
	}
	appendBucketAmountRows(rows, orderType, amounts)
	return nil
}

func accumulateTopUpBucketRows(query *gorm.DB, timestampColumn string, loc *time.Location, revenueRange *orderRevenueRange, amounts map[string]int64) error {
	rows, err := queryBucketAmountRows(query, timestampColumn, topUpAmountFenExpr(), loc, revenueRange)
	if err != nil {
		return err
	}
	appendBucketAmountRows(rows, orderTypeTopup, amounts)
	return nil
}

func queryBucketAmountRows(query *gorm.DB, timestampColumn string, amountExpr string, loc *time.Location, revenueRange *orderRevenueRange) ([]bucketAmountRow, error) {
	bucketExpr, vars, supported := orderRevenueBucketExpr(timestampColumn, loc, revenueRange)
	if supported {
		var rows []bucketAmountRow
		selectExpr := fmt.Sprintf("%s AS bucket_key, COALESCE(SUM(%s), 0) AS amount_fen", bucketExpr, amountExpr)
		if err := query.Select(selectExpr, vars...).Group("bucket_key").Order("bucket_key ASC").Scan(&rows).Error; err != nil {
			return nil, err
		}
		return rows, nil
	}
	return queryBucketAmountRowsFallback(query, timestampColumn, amountExpr, loc, revenueRange)
}

func queryBucketAmountRowsFallback(query *gorm.DB, timestampColumn string, amountExpr string, loc *time.Location, revenueRange *orderRevenueRange) ([]bucketAmountRow, error) {
	rows, err := query.Select(fmt.Sprintf("%s AS ts, %s AS amount_fen", timestampColumn, amountExpr)).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	amounts := make(map[string]int64)
	for rows.Next() {
		var row rawBucketAmountRow
		if err := DB.ScanRows(rows, &row); err != nil {
			return nil, err
		}
		if row.Timestamp <= 0 || row.AmountFen <= 0 {
			continue
		}
		bucketKey := orderRevenueBucketKeyForTime(time.Unix(row.Timestamp, 0).In(loc), revenueRange)
		if bucketKey == "" {
			continue
		}
		amounts[bucketKey] += row.AmountFen
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]bucketAmountRow, 0, len(amounts))
	for _, bucketKey := range revenueRange.BucketKeys {
		if amountFen, ok := amounts[bucketKey]; ok {
			out = append(out, bucketAmountRow{
				BucketKey: bucketKey,
				AmountFen: amountFen,
			})
		}
	}
	return out, nil
}

func appendBucketAmountRows(rows []bucketAmountRow, orderType string, amounts map[string]int64) {
	for _, row := range rows {
		if row.BucketKey == "" || row.AmountFen <= 0 {
			continue
		}
		amounts[orderRevenueAmountKey(row.BucketKey, orderType)] += row.AmountFen
	}
}

func orderRevenueAmountKey(bucketKey string, orderType string) string {
	return bucketKey + "__" + orderType
}

func orderPaidTimestampExpr(paidAtColumn string, finishedAtColumn string) string {
	return fmt.Sprintf("COALESCE(NULLIF(%s, 0), %s)", paidAtColumn, finishedAtColumn)
}

func nonZeroTimestampExpr(column string) string {
	return fmt.Sprintf("NULLIF(%s, 0)", column)
}

func orderRevenueBucketKeyForTime(bucketTime time.Time, revenueRange *orderRevenueRange) string {
	var bucketKey string
	switch revenueRange.Period {
	case "day":
		bucketKey = bucketTime.Format("15:00")
	case "week", "month":
		bucketKey = bucketTime.Format("2006-01-02")
	case "year":
		bucketKey = bucketTime.Format("2006-01")
	default:
		return ""
	}

	if _, ok := revenueRange.BucketLabels[bucketKey]; !ok {
		return ""
	}
	return bucketKey
}

func topUpAmountFenExpr() string {
	switch {
	case common.UsingPostgreSQL:
		return "COALESCE(NULLIF(payment_amount_fen, 0), CAST(ROUND(money * 100, 0) AS BIGINT))"
	case common.UsingSQLite:
		return "COALESCE(NULLIF(payment_amount_fen, 0), CAST(ROUND(money * 100, 0) AS INTEGER))"
	default:
		return "COALESCE(NULLIF(payment_amount_fen, 0), CAST(ROUND(money * 100, 0) AS SIGNED))"
	}
}

func orderRevenueBucketExpr(timestampColumn string, loc *time.Location, revenueRange *orderRevenueRange) (string, []interface{}, bool) {
	switch {
	case common.UsingSQLite:
		return sqliteOrderRevenueBucketExpr(timestampColumn, revenueRange.Period), nil, true
	case common.UsingPostgreSQL:
		if zone, ok := postgreSQLTimeZoneToken(loc, revenueRange.Start, revenueRange.End); ok {
			return postgreSQLOrderRevenueBucketExpr(timestampColumn, revenueRange.Period), []interface{}{zone}, true
		}
		return "", nil, false
	default:
		if offset, ok := stableTimeZoneOffsetToken(loc, revenueRange.Start, revenueRange.End); ok {
			return mySQLOrderRevenueBucketExpr(timestampColumn, revenueRange.Period), []interface{}{offset}, true
		}
		return "", nil, false
	}
}

func sqliteOrderRevenueBucketExpr(timestampColumn string, period string) string {
	switch period {
	case "day":
		return fmt.Sprintf("STRFTIME('%%H:00', DATETIME(%s, 'unixepoch', 'localtime'))", timestampColumn)
	case "week", "month":
		return fmt.Sprintf("DATE(%s, 'unixepoch', 'localtime')", timestampColumn)
	case "year":
		return fmt.Sprintf("STRFTIME('%%Y-%%m', DATETIME(%s, 'unixepoch', 'localtime'))", timestampColumn)
	default:
		return ""
	}
}

func postgreSQLOrderRevenueBucketExpr(timestampColumn string, period string) string {
	baseExpr := fmt.Sprintf("TO_TIMESTAMP(%s) AT TIME ZONE ?", timestampColumn)
	switch period {
	case "day":
		return fmt.Sprintf("TO_CHAR(%s, 'HH24:00')", baseExpr)
	case "week", "month":
		return fmt.Sprintf("TO_CHAR(%s, 'YYYY-MM-DD')", baseExpr)
	case "year":
		return fmt.Sprintf("TO_CHAR(%s, 'YYYY-MM')", baseExpr)
	default:
		return ""
	}
}

func mySQLOrderRevenueBucketExpr(timestampColumn string, period string) string {
	baseExpr := fmt.Sprintf("CONVERT_TZ(TIMESTAMPADD(SECOND, %s, '1970-01-01 00:00:00'), '+00:00', ?)", timestampColumn)
	switch period {
	case "day":
		return fmt.Sprintf("DATE_FORMAT(%s, '%%H:00')", baseExpr)
	case "week", "month":
		return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m-%%d')", baseExpr)
	case "year":
		return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m')", baseExpr)
	default:
		return ""
	}
}

func postgreSQLTimeZoneToken(loc *time.Location, rangeStart time.Time, rangeEnd time.Time) (string, bool) {
	if loc == nil {
		return "", false
	}
	zone := loc.String()
	if zone != "" && zone != "Local" {
		return zone, true
	}
	return stableTimeZoneOffsetToken(loc, rangeStart, rangeEnd)
}

func stableTimeZoneOffsetToken(loc *time.Location, rangeStart time.Time, rangeEnd time.Time) (string, bool) {
	if loc == nil {
		return "", false
	}
	_, baseOffset := rangeStart.In(loc).Zone()
	sampleEnd := rangeEnd
	if !sampleEnd.After(rangeStart) {
		sampleEnd = rangeStart.Add(time.Second)
	}
	for sample := rangeStart; sample.Before(sampleEnd); sample = sample.Add(6 * time.Hour) {
		_, offset := sample.In(loc).Zone()
		if offset != baseOffset {
			return "", false
		}
	}
	last := rangeEnd.Add(-time.Second)
	if last.After(rangeStart) {
		_, offset := last.In(loc).Zone()
		if offset != baseOffset {
			return "", false
		}
	}
	return formatUTCOffset(baseOffset), true
}

func formatUTCOffset(offsetSeconds int) string {
	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}
	return fmt.Sprintf("%s%02d:%02d", sign, offsetSeconds/3600, (offsetSeconds%3600)/60)
}
