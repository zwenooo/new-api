package model

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type ServiceStatusConsumeAggRow struct {
	GroupCode   string `gorm:"column:group_code"`
	BucketStart int64  `gorm:"column:bucket_start"`
	Success     int64  `gorm:"column:success_count"`
}

type ServiceStatusErrorAggRow struct {
	GroupCode        string `gorm:"column:group_code"`
	BucketStart      int64  `gorm:"column:bucket_start"`
	ServerErrors     int64  `gorm:"column:server_error_count"`
	ClientDisconnect int64  `gorm:"column:client_abort_count"`
	TotalErrors      int64  `gorm:"column:total_error_count"`
}

type ServiceStatusAggRow struct {
	GroupCode     string `gorm:"column:group_code"`
	BucketStart   int64  `gorm:"column:bucket_start"`
	Success       int64  `gorm:"column:success_count"`
	ServerErrors  int64  `gorm:"column:server_error_count"`
	ClientAborts  int64  `gorm:"column:client_abort_count"`
	BucketSeconds int64  `gorm:"column:bucket_seconds"`
}

type serviceStatusRequestKey struct {
	GroupCode string
	RequestID string
}

type serviceStatusGroupBucketKey struct {
	GroupCode   string
	BucketStart int64
}

type serviceStatusLatestRequestAgg struct {
	GroupCode  string
	CreatedAt  int64
	ID         int
	FinalState int
}

func ListServiceStatusConsumeAgg(
	ctx context.Context,
	start int64,
	end int64,
	bucketSeconds int64,
	groupCodes []string,
) ([]ServiceStatusConsumeAggRow, error) {
	if start <= 0 {
		return nil, fmt.Errorf("start 无效")
	}
	if end <= 0 {
		return nil, fmt.Errorf("end 无效")
	}
	if start >= end {
		return nil, fmt.Errorf("start 必须小于 end")
	}
	if bucketSeconds <= 0 {
		return nil, fmt.Errorf("bucketSeconds 无效")
	}

	mode, keywords, _ := getServiceStatusUAFilter()
	if mode == serviceStatusUAFilterModeInclude && len(keywords) == 0 {
		return []ServiceStatusConsumeAggRow{}, nil
	}

	bucketExpr := fmt.Sprintf("(created_at - (created_at %% %d))", bucketSeconds)
	groupExpr := logGroupCol

	q := LOG_DB.WithContext(ctx).Table("logs").
		Select(fmt.Sprintf("%s AS group_code, %s AS bucket_start, COUNT(*) AS success_count", groupExpr, bucketExpr)).
		Where("type = ?", LogTypeConsume).
		Where("created_at >= ? AND created_at < ?", start, end).
		Where(fmt.Sprintf("%s <> ''", groupExpr)).
		Group(fmt.Sprintf("%s, %s", groupExpr, bucketExpr))

	uaWhere, uaArgs := buildServiceStatusUAFilterWhere(mode, keywords)
	if uaWhere != "" {
		q = q.Where(uaWhere, uaArgs...)
	}

	if len(groupCodes) > 0 {
		q = q.Where(fmt.Sprintf("%s IN ?", groupExpr), groupCodes)
	}

	rows := make([]ServiceStatusConsumeAggRow, 0)
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func ListServiceStatusErrorAgg(
	ctx context.Context,
	start int64,
	end int64,
	bucketSeconds int64,
	groupCodes []string,
) ([]ServiceStatusErrorAggRow, error) {
	if start <= 0 {
		return nil, fmt.Errorf("start 无效")
	}
	if end <= 0 {
		return nil, fmt.Errorf("end 无效")
	}
	if start >= end {
		return nil, fmt.Errorf("start 必须小于 end")
	}
	if bucketSeconds <= 0 {
		return nil, fmt.Errorf("bucketSeconds 无效")
	}

	mode, keywords, _ := getServiceStatusUAFilter()
	if mode == serviceStatusUAFilterModeInclude && len(keywords) == 0 {
		return []ServiceStatusErrorAggRow{}, nil
	}

	bucketExpr := fmt.Sprintf("(created_at - (created_at %% %d))", bucketSeconds)
	groupExpr := logGroupCol

	clientAbortCond := `(other LIKE '%"stream_exit_reason":"client_disconnected"%' OR content LIKE '%context canceled%' OR content LIKE '%broken pipe%' OR content LIKE '%connection reset by peer%')`
	serverErrorCoreCond := `(other LIKE '%"status_code":5%' OR (other NOT LIKE '%"status_code":%' AND (content LIKE '%Response status: 5%' OR content LIKE 'Response processing failed:%' OR (content LIKE 'Request failed:%' AND content NOT LIKE '%context canceled%'))))`
	serverErrorCond := fmt.Sprintf("(NOT (%s) AND (%s))", clientAbortCond, serverErrorCoreCond)

	selectExpr := fmt.Sprintf(
		"%s AS group_code, %s AS bucket_start, "+
			"SUM(CASE WHEN %s THEN 1 ELSE 0 END) AS server_error_count, "+
			"SUM(CASE WHEN %s THEN 1 ELSE 0 END) AS client_abort_count, "+
			"COUNT(*) AS total_error_count",
		groupExpr,
		bucketExpr,
		serverErrorCond,
		clientAbortCond,
	)

	q := LOG_DB.WithContext(ctx).Table("logs").
		Select(selectExpr).
		Where("type = ?", LogTypeError).
		Where("created_at >= ? AND created_at < ?", start, end).
		Where(fmt.Sprintf("%s <> ''", groupExpr)).
		Group(fmt.Sprintf("%s, %s", groupExpr, bucketExpr))

	uaWhere, uaArgs := buildServiceStatusUAFilterWhere(mode, keywords)
	if uaWhere != "" {
		q = q.Where(uaWhere, uaArgs...)
	}

	if len(groupCodes) > 0 {
		q = q.Where(fmt.Sprintf("%s IN ?", groupExpr), groupCodes)
	}

	rows := make([]ServiceStatusErrorAggRow, 0)
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func ListServiceStatusAgg(
	ctx context.Context,
	start int64,
	end int64,
	bucketSeconds int64,
	groupCodes []string,
) ([]ServiceStatusAggRow, error) {
	if start <= 0 {
		return nil, fmt.Errorf("start 无效")
	}
	if end <= 0 {
		return nil, fmt.Errorf("end 无效")
	}
	if start >= end {
		return nil, fmt.Errorf("start 必须小于 end")
	}
	if bucketSeconds <= 0 {
		return nil, fmt.Errorf("bucketSeconds 无效")
	}

	requestRows, err := listServiceStatusFinalRequestAgg(ctx, start, end, bucketSeconds, groupCodes)
	if err != nil {
		return nil, err
	}
	legacyRows, err := listServiceStatusLegacyAggWithoutRequestID(ctx, start, end, bucketSeconds, groupCodes)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]*ServiceStatusAggRow, len(requestRows)+len(legacyRows))
	mergeRows := func(rows []ServiceStatusAggRow) {
		for _, row := range rows {
			key := row.GroupCode + ":" + strconv.FormatInt(row.BucketStart, 10)
			existing, ok := merged[key]
			if !ok {
				copyRow := row
				copyRow.BucketSeconds = bucketSeconds
				merged[key] = &copyRow
				continue
			}
			existing.Success += row.Success
			existing.ServerErrors += row.ServerErrors
			existing.ClientAborts += row.ClientAborts
		}
	}
	mergeRows(requestRows)
	mergeRows(legacyRows)

	rows := make([]ServiceStatusAggRow, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].GroupCode != rows[j].GroupCode {
			return rows[i].GroupCode < rows[j].GroupCode
		}
		return rows[i].BucketStart < rows[j].BucketStart
	})
	return rows, nil
}

func listServiceStatusFinalRequestAgg(
	ctx context.Context,
	start int64,
	end int64,
	bucketSeconds int64,
	groupCodes []string,
) ([]ServiceStatusAggRow, error) {
	mode, keywords, _ := getServiceStatusUAFilter()
	if mode == serviceStatusUAFilterModeInclude && len(keywords) == 0 {
		return []ServiceStatusAggRow{}, nil
	}

	groupExpr := logGroupCol

	q := LOG_DB.WithContext(ctx).Table("logs").
		Select(strings.Join([]string{
			"id",
			fmt.Sprintf("%s AS group_code", groupExpr),
			"request_id",
			"created_at",
			"type",
			"content",
			"other",
		}, ", ")).
		Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
		Where("created_at >= ? AND created_at < ?", start, end).
		Where(fmt.Sprintf("%s <> ''", groupExpr)).
		Where("COALESCE(request_id, '') <> ''")

	uaWhere, uaArgs := buildServiceStatusUAFilterWhere(mode, keywords)
	if uaWhere != "" {
		q = q.Where(uaWhere, uaArgs...)
	}

	if len(groupCodes) > 0 {
		q = q.Where(fmt.Sprintf("%s IN ?", groupExpr), groupCodes)
	}

	rows, err := q.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	latestByRequest := make(map[serviceStatusRequestKey]serviceStatusLatestRequestAgg)
	for rows.Next() {
		var row struct {
			ID        int
			GroupCode string
			RequestID string
			CreatedAt int64
			Type      int
			Content   string
			Other     string
		}
		if err := rows.Scan(&row.ID, &row.GroupCode, &row.RequestID, &row.CreatedAt, &row.Type, &row.Content, &row.Other); err != nil {
			return nil, err
		}

		if !serviceStatusOtherMatchesUAFilter(row.Other, mode, keywords) {
			continue
		}

		finalState := serviceStatusFinalStateForLog(row.Type, row.Content, row.Other)
		if finalState == serviceStatusRequestStateNone {
			continue
		}

		groupCode := strings.TrimSpace(row.GroupCode)
		requestID := strings.TrimSpace(row.RequestID)
		if groupCode == "" || requestID == "" || row.CreatedAt <= 0 {
			continue
		}

		key := serviceStatusRequestKey{
			GroupCode: groupCode,
			RequestID: requestID,
		}
		current, ok := latestByRequest[key]
		if ok && (current.CreatedAt > row.CreatedAt || (current.CreatedAt == row.CreatedAt && current.ID >= row.ID)) {
			continue
		}
		latestByRequest[key] = serviceStatusLatestRequestAgg{
			GroupCode:  groupCode,
			CreatedAt:  row.CreatedAt,
			ID:         row.ID,
			FinalState: finalState,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	aggMap := make(map[serviceStatusGroupBucketKey]*ServiceStatusAggRow, len(latestByRequest))
	for _, row := range latestByRequest {
		bucketStart := row.CreatedAt - (row.CreatedAt % bucketSeconds)
		if bucketStart <= 0 {
			continue
		}

		key := serviceStatusGroupBucketKey{
			GroupCode:   row.GroupCode,
			BucketStart: bucketStart,
		}
		aggRow, ok := aggMap[key]
		if !ok {
			aggRow = &ServiceStatusAggRow{
				GroupCode:     row.GroupCode,
				BucketStart:   bucketStart,
				BucketSeconds: bucketSeconds,
			}
			aggMap[key] = aggRow
		}

		switch row.FinalState {
		case serviceStatusRequestStateSuccess:
			aggRow.Success++
		case serviceStatusRequestStateServerError:
			aggRow.ServerErrors++
		case serviceStatusRequestStateClientAbort:
			aggRow.ClientAborts++
		}
	}

	aggRows := make([]ServiceStatusAggRow, 0, len(aggMap))
	for _, row := range aggMap {
		aggRows = append(aggRows, *row)
	}
	sort.Slice(aggRows, func(i, j int) bool {
		if aggRows[i].GroupCode != aggRows[j].GroupCode {
			return aggRows[i].GroupCode < aggRows[j].GroupCode
		}
		return aggRows[i].BucketStart < aggRows[j].BucketStart
	})
	return aggRows, nil
}

func listServiceStatusLegacyAggWithoutRequestID(
	ctx context.Context,
	start int64,
	end int64,
	bucketSeconds int64,
	groupCodes []string,
) ([]ServiceStatusAggRow, error) {
	mode, keywords, _ := getServiceStatusUAFilter()
	if mode == serviceStatusUAFilterModeInclude && len(keywords) == 0 {
		return []ServiceStatusAggRow{}, nil
	}

	bucketExpr := fmt.Sprintf("(created_at - (created_at %% %d))", bucketSeconds)
	groupExpr := logGroupCol

	clientAbortCond := `(other LIKE '%"stream_exit_reason":"client_disconnected"%' OR content LIKE '%context canceled%' OR content LIKE '%broken pipe%' OR content LIKE '%connection reset by peer%')`
	serverErrorCoreCond := `(other LIKE '%"status_code":5%' OR (other NOT LIKE '%"status_code":%' AND (content LIKE '%Response status: 5%' OR content LIKE 'Response processing failed:%' OR (content LIKE 'Request failed:%' AND content NOT LIKE '%context canceled%'))))`
	serverErrorCond := fmt.Sprintf("(NOT (%s) AND (%s))", clientAbortCond, serverErrorCoreCond)

	selectExpr := strings.Join([]string{
		fmt.Sprintf("%s AS group_code", groupExpr),
		fmt.Sprintf("%s AS bucket_start", bucketExpr),
		fmt.Sprintf("SUM(CASE WHEN type = %d THEN 1 ELSE 0 END) AS success_count", LogTypeConsume),
		fmt.Sprintf("SUM(CASE WHEN type = %d AND %s THEN 1 ELSE 0 END) AS server_error_count", LogTypeError, serverErrorCond),
		fmt.Sprintf("SUM(CASE WHEN type = %d AND %s THEN 1 ELSE 0 END) AS client_abort_count", LogTypeError, clientAbortCond),
	}, ", ")

	q := LOG_DB.WithContext(ctx).Table("logs").
		Select(selectExpr).
		Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
		Where("created_at >= ? AND created_at < ?", start, end).
		Where(fmt.Sprintf("%s <> ''", groupExpr)).
		Where("COALESCE(request_id, '') = ''").
		Group(fmt.Sprintf("%s, %s", groupExpr, bucketExpr))

	uaWhere, uaArgs := buildServiceStatusUAFilterWhere(mode, keywords)
	if uaWhere != "" {
		q = q.Where(uaWhere, uaArgs...)
	}

	if len(groupCodes) > 0 {
		q = q.Where(fmt.Sprintf("%s IN ?", groupExpr), groupCodes)
	}

	rows := make([]ServiceStatusAggRow, 0)
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].BucketSeconds = bucketSeconds
	}
	return rows, nil
}
