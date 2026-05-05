package model

import (
	"context"
	"errors"
	"fmt"
	"one-api/common"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	serviceStatusBucketSecondsMinute int64 = 60
	serviceStatusBucketSecondsHour   int64 = 3600
	serviceStatusBucketSecondsDay    int64 = 86400

	serviceStatusRequestStateNone        = 0
	serviceStatusRequestStateClientAbort = 1
	serviceStatusRequestStateServerError = 2
	serviceStatusRequestStateSuccess     = 3

	serviceStatusStatsAlgorithmVersion = "request-final-v2"
)

// ServiceStatusBucketStat stores aggregated request results per group and time bucket.
//
// Note:
//   - This table is stored in LOG_DB (same DB as logs).
//   - It is updated incrementally when logs are written/updated, so timeline queries
//     avoid scanning the (potentially huge) logs table.
type ServiceStatusBucketStat struct {
	BucketSeconds int64  `json:"bucket_seconds" gorm:"primaryKey;autoIncrement:false;column:bucket_seconds"`
	BucketStart   int64  `json:"bucket_start" gorm:"primaryKey;autoIncrement:false;column:bucket_start;index"`
	GroupCode     string `json:"group_code" gorm:"primaryKey;autoIncrement:false;column:group_code;type:varchar(64);index"`

	SuccessCount     int64 `json:"success_count" gorm:"column:success_count;not null;default:0"`
	ServerErrorCount int64 `json:"server_error_count" gorm:"column:server_error_count;not null;default:0"`
	ClientAbortCount int64 `json:"client_abort_count" gorm:"column:client_abort_count;not null;default:0"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ServiceStatusBucketStat) TableName() string {
	return "service_status_bucket_stats"
}

// ServiceStatusRequestState stores the user-visible final outcome of a request within a group.
// Multiple retry attempts for the same request_id/group_code collapse into a single terminal state.
type ServiceStatusRequestState struct {
	RequestId  string `json:"request_id" gorm:"primaryKey;autoIncrement:false;column:request_id;type:varchar(64)"`
	GroupCode  string `json:"group_code" gorm:"primaryKey;autoIncrement:false;column:group_code;type:varchar(64);index"`
	EventAt    int64  `json:"event_at" gorm:"column:event_at;not null;default:0;index"`
	FinalState int    `json:"final_state" gorm:"column:final_state;not null;default:0"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ServiceStatusRequestState) TableName() string {
	return "service_status_request_states"
}

// ServiceStatusBucketStatsMeta tracks the earliest backfilled bucket start for a given bucket size.
// It prevents re-scanning historical logs on each page view.
type ServiceStatusBucketStatsMeta struct {
	BucketSeconds   int64  `json:"bucket_seconds" gorm:"primaryKey;autoIncrement:false;column:bucket_seconds"`
	BackfilledStart int64  `json:"backfilled_start" gorm:"column:backfilled_start;not null;default:0"`
	UAFilterHash    string `json:"ua_filter_hash" gorm:"column:ua_filter_hash;type:varchar(64);not null;default:''"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (ServiceStatusBucketStatsMeta) TableName() string {
	return "service_status_bucket_stats_meta"
}

func serviceStatusBucketStart(ts int64, bucketSeconds int64) int64 {
	if ts <= 0 || bucketSeconds <= 0 {
		return 0
	}
	return ts - ts%bucketSeconds
}

func getServiceStatusStatsConfigHash() string {
	mode, keywords, _ := getServiceStatusUAFilter()
	return hashServiceStatusUAFilter(serviceStatusStatsAlgorithmVersion+":"+mode, keywords)
}

func isServiceStatusClientAbort(content string, other string) bool {
	if strings.Contains(other, `"stream_exit_reason":"client_disconnected"`) {
		return true
	}
	lower := strings.ToLower(content)
	return strings.Contains(lower, "context canceled") ||
		strings.Contains(lower, "broken pipe") ||
		strings.Contains(lower, "connection reset by peer")
}

func isServiceStatusServerError(content string, other string) bool {
	if isServiceStatusClientAbort(content, other) {
		return false
	}

	statusCode, ok := extractServiceStatusStatusCode(other)
	if ok {
		return statusCode/100 == 5
	}

	if strings.Contains(content, "Response status: 5") {
		return true
	}
	if strings.HasPrefix(content, "Response processing failed:") {
		return true
	}
	if strings.HasPrefix(content, "Request failed:") {
		// 已在 client abort 中排除 context canceled 等场景
		return true
	}
	return false
}

func extractServiceStatusStatusCode(other string) (int, bool) {
	idx := strings.Index(other, `"status_code":`)
	if idx < 0 {
		return 0, false
	}
	i := idx + len(`"status_code":`)
	for i < len(other) {
		switch other[i] {
		case ' ', '\t', '\n', '\r':
			i++
			continue
		default:
		}
		break
	}
	j := i
	for j < len(other) && other[j] >= '0' && other[j] <= '9' {
		j++
	}
	if j == i {
		return 0, false
	}
	code, err := strconv.Atoi(other[i:j])
	if err != nil {
		return 0, false
	}
	return code, true
}

func serviceStatusFinalStateForLog(logType int, content string, other string) int {
	switch logType {
	case LogTypeConsume:
		return serviceStatusRequestStateSuccess
	case LogTypeError:
		if isServiceStatusClientAbort(content, other) {
			return serviceStatusRequestStateClientAbort
		}
		if isServiceStatusServerError(content, other) {
			return serviceStatusRequestStateServerError
		}
	}
	return serviceStatusRequestStateNone
}

func serviceStatusCountsForLog(logType int, content string, other string) (success int64, serverErr int64, clientAbort int64) {
	switch logType {
	case LogTypeConsume, LogTypeError:
	default:
		return 0, 0, 0
	}

	mode, keywords, _ := getServiceStatusUAFilter()
	if !serviceStatusOtherMatchesUAFilter(other, mode, keywords) {
		return 0, 0, 0
	}

	switch serviceStatusFinalStateForLog(logType, content, other) {
	case serviceStatusRequestStateSuccess:
		return 1, 0, 0
	case serviceStatusRequestStateServerError:
		return 0, 1, 0
	case serviceStatusRequestStateClientAbort:
		return 0, 0, 1
	default:
		return 0, 0, 0
	}
}

func upsertServiceStatusBucketStatExact(ctx context.Context, row ServiceStatusBucketStat) error {
	if row.BucketSeconds <= 0 || row.BucketStart <= 0 || strings.TrimSpace(row.GroupCode) == "" {
		return errors.New("service_status_bucket_stats 参数无效")
	}
	tx := LOG_DB.WithContext(ctx)
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "bucket_seconds"},
			{Name: "bucket_start"},
			{Name: "group_code"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"success_count":      row.SuccessCount,
			"server_error_count": row.ServerErrorCount,
			"client_abort_count": row.ClientAbortCount,
			"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(&row).Error
}

const serviceStatusBucketStatExactUpsertBatchSize = 500

func upsertServiceStatusBucketStatExactRows(ctx context.Context, rows []ServiceStatusBucketStat) error {
	if len(rows) == 0 {
		return nil
	}

	var successExpr interface{}
	var serverExpr interface{}
	var clientExpr interface{}

	switch strings.ToLower(LOG_DB.Dialector.Name()) {
	case common.DatabaseTypeMySQL:
		successExpr = gorm.Expr("VALUES(success_count)")
		serverExpr = gorm.Expr("VALUES(server_error_count)")
		clientExpr = gorm.Expr("VALUES(client_abort_count)")
	default:
		// SQLite/PostgreSQL: use excluded.*
		successExpr = gorm.Expr("excluded.success_count")
		serverExpr = gorm.Expr("excluded.server_error_count")
		clientExpr = gorm.Expr("excluded.client_abort_count")
	}

	assignments := map[string]interface{}{
		"success_count":      successExpr,
		"server_error_count": serverExpr,
		"client_abort_count": clientExpr,
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
	}

	tx := LOG_DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "bucket_seconds"},
			{Name: "bucket_start"},
			{Name: "group_code"},
		},
		DoUpdates: clause.Assignments(assignments),
	})

	for i := 0; i < len(rows); i += serviceStatusBucketStatExactUpsertBatchSize {
		j := i + serviceStatusBucketStatExactUpsertBatchSize
		if j > len(rows) {
			j = len(rows)
		}
		batch := rows[i:j]
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
	}
	return nil
}

func bumpServiceStatusBucketStatDelta(ctx context.Context, bucketSeconds int64, bucketStart int64, groupCode string, deltaSuccess int64, deltaServerErr int64, deltaClientAbort int64) error {
	groupCode = strings.TrimSpace(groupCode)
	if bucketSeconds <= 0 || bucketStart <= 0 || groupCode == "" {
		return errors.New("service_status_bucket_stats 参数无效")
	}
	if deltaSuccess == 0 && deltaServerErr == 0 && deltaClientAbort == 0 {
		return nil
	}
	row := ServiceStatusBucketStat{
		BucketSeconds:    bucketSeconds,
		BucketStart:      bucketStart,
		GroupCode:        groupCode,
		SuccessCount:     deltaSuccess,
		ServerErrorCount: deltaServerErr,
		ClientAbortCount: deltaClientAbort,
	}
	tx := LOG_DB.WithContext(ctx)
	assignments := map[string]interface{}{}
	if deltaSuccess != 0 {
		assignments["success_count"] = gorm.Expr("success_count + ?", deltaSuccess)
	}
	if deltaServerErr != 0 {
		assignments["server_error_count"] = gorm.Expr("server_error_count + ?", deltaServerErr)
	}
	if deltaClientAbort != 0 {
		assignments["client_abort_count"] = gorm.Expr("client_abort_count + ?", deltaClientAbort)
	}
	assignments["updated_at"] = gorm.Expr("CURRENT_TIMESTAMP")

	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "bucket_seconds"},
			{Name: "bucket_start"},
			{Name: "group_code"},
		},
		DoUpdates: clause.Assignments(assignments),
	}).Create(&row).Error
}

type serviceStatusBucketStatDeltaKey struct {
	BucketSeconds int64
	BucketStart   int64
	GroupCode     string
}

type serviceStatusBucketStatDelta struct {
	SuccessCount     int64
	ServerErrorCount int64
	ClientAbortCount int64
}

type serviceStatusBucketStatDeltaBuffer struct {
	mu      sync.Mutex
	started bool
	wakeCh  chan struct{}
	deltas  map[serviceStatusBucketStatDeltaKey]serviceStatusBucketStatDelta
}

var serviceStatusBucketStatDeltas serviceStatusBucketStatDeltaBuffer
var serviceStatusBucketStatFlushMu sync.Mutex

const (
	serviceStatusBucketStatFlushInterval = 5 * time.Second
	serviceStatusBucketStatFlushTimeout  = 5 * time.Second
	serviceStatusBucketStatWakeThreshold = 2000
	serviceStatusBucketStatMaxPendingKey = 20000
)

func bumpServiceStatusBucketStatDeltas(ctx context.Context, rows []ServiceStatusBucketStat) error {
	if len(rows) == 0 {
		return nil
	}

	var successExpr interface{}
	var serverExpr interface{}
	var clientExpr interface{}

	switch strings.ToLower(LOG_DB.Dialector.Name()) {
	case common.DatabaseTypeMySQL:
		successExpr = gorm.Expr("success_count + VALUES(success_count)")
		serverExpr = gorm.Expr("server_error_count + VALUES(server_error_count)")
		clientExpr = gorm.Expr("client_abort_count + VALUES(client_abort_count)")
	default:
		// SQLite/PostgreSQL: use excluded.*
		successExpr = gorm.Expr("success_count + excluded.success_count")
		serverExpr = gorm.Expr("server_error_count + excluded.server_error_count")
		clientExpr = gorm.Expr("client_abort_count + excluded.client_abort_count")
	}

	assignments := map[string]interface{}{
		"success_count":      successExpr,
		"server_error_count": serverExpr,
		"client_abort_count": clientExpr,
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
	}

	tx := LOG_DB.WithContext(ctx)
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "bucket_seconds"},
			{Name: "bucket_start"},
			{Name: "group_code"},
		},
		DoUpdates: clause.Assignments(assignments),
	}).Create(&rows).Error
}

func ensureServiceStatusBucketStatDeltaFlusherStartedLocked() {
	if serviceStatusBucketStatDeltas.started {
		return
	}
	serviceStatusBucketStatDeltas.started = true
	serviceStatusBucketStatDeltas.wakeCh = make(chan struct{}, 1)
	serviceStatusBucketStatDeltas.deltas = make(map[serviceStatusBucketStatDeltaKey]serviceStatusBucketStatDelta)

	wakeCh := serviceStatusBucketStatDeltas.wakeCh
	go func() {
		ticker := time.NewTicker(serviceStatusBucketStatFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				flushServiceStatusBucketStatDeltas()
			case <-wakeCh:
				flushServiceStatusBucketStatDeltas()
			}
		}
	}()
}

func enqueueServiceStatusBucketStatDelta(bucketSeconds int64, bucketStart int64, groupCode string, deltaSuccess int64, deltaServerErr int64, deltaClientAbort int64) error {
	groupCode = strings.TrimSpace(groupCode)
	if bucketSeconds <= 0 || bucketStart <= 0 || groupCode == "" {
		return errors.New("service_status_bucket_stats 参数无效")
	}
	if deltaSuccess == 0 && deltaServerErr == 0 && deltaClientAbort == 0 {
		return nil
	}

	serviceStatusBucketStatDeltas.mu.Lock()
	ensureServiceStatusBucketStatDeltaFlusherStartedLocked()

	key := serviceStatusBucketStatDeltaKey{
		BucketSeconds: bucketSeconds,
		BucketStart:   bucketStart,
		GroupCode:     groupCode,
	}
	delta := serviceStatusBucketStatDeltas.deltas[key]
	delta.SuccessCount += deltaSuccess
	delta.ServerErrorCount += deltaServerErr
	delta.ClientAbortCount += deltaClientAbort

	if delta.SuccessCount == 0 && delta.ServerErrorCount == 0 && delta.ClientAbortCount == 0 {
		delete(serviceStatusBucketStatDeltas.deltas, key)
	} else {
		serviceStatusBucketStatDeltas.deltas[key] = delta
	}

	pendingKeys := len(serviceStatusBucketStatDeltas.deltas)
	wakeCh := serviceStatusBucketStatDeltas.wakeCh
	serviceStatusBucketStatDeltas.mu.Unlock()

	if pendingKeys >= serviceStatusBucketStatWakeThreshold {
		select {
		case wakeCh <- struct{}{}:
		default:
		}
	}
	return nil
}

func flushServiceStatusBucketStatDeltas() {
	serviceStatusBucketStatFlushMu.Lock()
	defer serviceStatusBucketStatFlushMu.Unlock()

	serviceStatusBucketStatDeltas.mu.Lock()
	if len(serviceStatusBucketStatDeltas.deltas) == 0 {
		serviceStatusBucketStatDeltas.mu.Unlock()
		return
	}
	deltas := serviceStatusBucketStatDeltas.deltas
	serviceStatusBucketStatDeltas.deltas = make(map[serviceStatusBucketStatDeltaKey]serviceStatusBucketStatDelta)
	serviceStatusBucketStatDeltas.mu.Unlock()

	rows := make([]ServiceStatusBucketStat, 0, len(deltas))
	for key, delta := range deltas {
		if delta.SuccessCount == 0 && delta.ServerErrorCount == 0 && delta.ClientAbortCount == 0 {
			continue
		}
		rows = append(rows, ServiceStatusBucketStat{
			BucketSeconds:    key.BucketSeconds,
			BucketStart:      key.BucketStart,
			GroupCode:        key.GroupCode,
			SuccessCount:     delta.SuccessCount,
			ServerErrorCount: delta.ServerErrorCount,
			ClientAbortCount: delta.ClientAbortCount,
			CreatedAt:        time.Time{},
			UpdatedAt:        time.Time{},
		})
	}
	if len(rows) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), serviceStatusBucketStatFlushTimeout)
	err := bumpServiceStatusBucketStatDeltas(ctx, rows)
	cancel()
	if err == nil {
		return
	}

	common.SysLog(fmt.Sprintf("service status bucket stats flush failed: %v", err))

	serviceStatusBucketStatDeltas.mu.Lock()
	defer serviceStatusBucketStatDeltas.mu.Unlock()
	ensureServiceStatusBucketStatDeltaFlusherStartedLocked()
	for _, row := range rows {
		key := serviceStatusBucketStatDeltaKey{
			BucketSeconds: row.BucketSeconds,
			BucketStart:   row.BucketStart,
			GroupCode:     row.GroupCode,
		}
		delta := serviceStatusBucketStatDeltas.deltas[key]
		delta.SuccessCount += row.SuccessCount
		delta.ServerErrorCount += row.ServerErrorCount
		delta.ClientAbortCount += row.ClientAbortCount
		if delta.SuccessCount == 0 && delta.ServerErrorCount == 0 && delta.ClientAbortCount == 0 {
			delete(serviceStatusBucketStatDeltas.deltas, key)
		} else {
			serviceStatusBucketStatDeltas.deltas[key] = delta
		}
	}
	if len(serviceStatusBucketStatDeltas.deltas) > serviceStatusBucketStatMaxPendingKey {
		serviceStatusBucketStatDeltas.deltas = make(map[serviceStatusBucketStatDeltaKey]serviceStatusBucketStatDelta)
		common.SysLog("service status bucket stats pending deltas dropped due to oversized buffer")
	}
}

func applyServiceStatusBucketStatDelta(ctx context.Context, createdAt int64, groupCode string, deltaSuccess int64, deltaServerErr int64, deltaClientAbort int64) error {
	_ = ctx
	groupCode = strings.TrimSpace(groupCode)
	if createdAt <= 0 || groupCode == "" {
		return nil
	}

	minuteStart := serviceStatusBucketStart(createdAt, serviceStatusBucketSecondsMinute)
	hourStart := serviceStatusBucketStart(createdAt, serviceStatusBucketSecondsHour)
	// day bucket is derived from hour bucket stats when queried/backfilled
	// to reduce DB write pressure.
	// dayStart := serviceStatusBucketStart(createdAt, serviceStatusBucketSecondsDay)
	if minuteStart > 0 {
		if err := enqueueServiceStatusBucketStatDelta(serviceStatusBucketSecondsMinute, minuteStart, groupCode, deltaSuccess, deltaServerErr, deltaClientAbort); err != nil {
			return err
		}
	}
	if hourStart > 0 {
		if err := enqueueServiceStatusBucketStatDelta(serviceStatusBucketSecondsHour, hourStart, groupCode, deltaSuccess, deltaServerErr, deltaClientAbort); err != nil {
			return err
		}
	}
	return nil
}

type serviceStatusRequestLogEvent struct {
	Id        int    `gorm:"column:id"`
	CreatedAt int64  `gorm:"column:created_at"`
	Type      int    `gorm:"column:type"`
	Content   string `gorm:"column:content"`
	Other     string `gorm:"column:other"`
}

func listServiceStatusRequestLogEvents(ctx context.Context, requestID string, groupCode string) ([]serviceStatusRequestLogEvent, error) {
	requestID = strings.TrimSpace(requestID)
	groupCode = strings.TrimSpace(groupCode)
	if requestID == "" || groupCode == "" {
		return nil, nil
	}

	mode, keywords, _ := getServiceStatusUAFilter()
	if mode == serviceStatusUAFilterModeInclude && len(keywords) == 0 {
		return nil, nil
	}

	q := LOG_DB.WithContext(ctx).Table("logs").
		Select("id, created_at, type, content, other").
		Where("request_id = ?", requestID).
		Where(fmt.Sprintf("%s = ?", logGroupCol), groupCode).
		Where("type IN ?", []int{LogTypeConsume, LogTypeError})

	uaWhere, uaArgs := buildServiceStatusUAFilterWhere(mode, keywords)
	if uaWhere != "" {
		q = q.Where(uaWhere, uaArgs...)
	}

	rows := make([]serviceStatusRequestLogEvent, 0, 4)
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func serviceStatusRequestStateFromLogEvents(rows []serviceStatusRequestLogEvent) (int, int64) {
	latestState := serviceStatusRequestStateNone
	latestAt := int64(0)
	latestID := 0

	for _, row := range rows {
		if row.CreatedAt <= 0 {
			continue
		}
		state := serviceStatusFinalStateForLog(row.Type, row.Content, row.Other)
		if state == serviceStatusRequestStateNone {
			continue
		}
		if row.CreatedAt > latestAt || (row.CreatedAt == latestAt && row.Id > latestID) {
			latestState = state
			latestAt = row.CreatedAt
			latestID = row.Id
		}
	}

	if latestState == serviceStatusRequestStateNone {
		return serviceStatusRequestStateNone, 0
	}
	return latestState, latestAt
}

func applyServiceStatusRequestStateCount(ctx context.Context, groupCode string, eventAt int64, state int, delta int64) error {
	if delta == 0 || state == serviceStatusRequestStateNone || eventAt <= 0 || strings.TrimSpace(groupCode) == "" {
		return nil
	}

	switch state {
	case serviceStatusRequestStateSuccess:
		return applyServiceStatusBucketStatDelta(ctx, eventAt, groupCode, delta, 0, 0)
	case serviceStatusRequestStateServerError:
		return applyServiceStatusBucketStatDelta(ctx, eventAt, groupCode, 0, delta, 0)
	case serviceStatusRequestStateClientAbort:
		return applyServiceStatusBucketStatDelta(ctx, eventAt, groupCode, 0, 0, delta)
	default:
		return nil
	}
}

func rebuildServiceStatusRequestState(ctx context.Context, requestID string, groupCode string) error {
	requestID = strings.TrimSpace(requestID)
	groupCode = strings.TrimSpace(groupCode)
	if requestID == "" || groupCode == "" {
		return nil
	}

	var prev ServiceStatusRequestState
	prevExists := false
	if err := LOG_DB.WithContext(ctx).Where("request_id = ? AND group_code = ?", requestID, groupCode).First(&prev).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	} else {
		prevExists = true
	}

	rows, err := listServiceStatusRequestLogEvents(ctx, requestID, groupCode)
	if err != nil {
		return err
	}
	nextState, nextEventAt := serviceStatusRequestStateFromLogEvents(rows)

	if prevExists {
		if err := applyServiceStatusRequestStateCount(ctx, prev.GroupCode, prev.EventAt, prev.FinalState, -1); err != nil {
			return err
		}
	}
	if nextState != serviceStatusRequestStateNone {
		if err := applyServiceStatusRequestStateCount(ctx, groupCode, nextEventAt, nextState, 1); err != nil {
			return err
		}
	}

	if nextState == serviceStatusRequestStateNone {
		if !prevExists {
			return nil
		}
		return LOG_DB.WithContext(ctx).Where("request_id = ? AND group_code = ?", requestID, groupCode).Delete(&ServiceStatusRequestState{}).Error
	}

	next := ServiceStatusRequestState{
		RequestId:  requestID,
		GroupCode:  groupCode,
		EventAt:    nextEventAt,
		FinalState: nextState,
	}
	return LOG_DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "request_id"},
			{Name: "group_code"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"event_at":    next.EventAt,
			"final_state": next.FinalState,
			"updated_at":  gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(&next).Error
}

// UpdateServiceStatusBucketStatsOnLogInsert increments stats for a newly inserted log row.
// It is best-effort and must never affect the main request flow.
func UpdateServiceStatusBucketStatsOnLogInsert(ctx context.Context, requestID string, logType int, createdAt int64, groupCode string, content string, other string) error {
	requestID = strings.TrimSpace(requestID)
	groupCode = strings.TrimSpace(groupCode)
	if requestID != "" && groupCode != "" {
		return rebuildServiceStatusRequestState(ctx, requestID, groupCode)
	}
	success, serverErr, clientAbort := serviceStatusCountsForLog(logType, content, other)
	return applyServiceStatusBucketStatDelta(ctx, createdAt, groupCode, success, serverErr, clientAbort)
}

// UpdateServiceStatusBucketStatsOnLogTypeChange adjusts stats when a log row changes type/content/other.
// Typical scenario: streaming logs transition from LogTypeConsumeInProgress -> LogTypeConsume/LogTypeError.
func UpdateServiceStatusBucketStatsOnLogTypeChange(ctx context.Context, requestID string, createdAt int64, oldGroupCode string, newGroupCode string, oldType int, oldContent string, oldOther string, newType int, newContent string, newOther string) error {
	requestID = strings.TrimSpace(requestID)
	oldGroupCode = strings.TrimSpace(oldGroupCode)
	newGroupCode = strings.TrimSpace(newGroupCode)

	if requestID != "" {
		if oldGroupCode != "" {
			if err := rebuildServiceStatusRequestState(ctx, requestID, oldGroupCode); err != nil {
				return err
			}
		}
		if newGroupCode != "" && newGroupCode != oldGroupCode {
			if err := rebuildServiceStatusRequestState(ctx, requestID, newGroupCode); err != nil {
				return err
			}
		}
		return nil
	}

	oldSuccess, oldServerErr, oldClientAbort := serviceStatusCountsForLog(oldType, oldContent, oldOther)
	newSuccess, newServerErr, newClientAbort := serviceStatusCountsForLog(newType, newContent, newOther)
	if oldGroupCode != "" && oldGroupCode != newGroupCode {
		if err := applyServiceStatusBucketStatDelta(ctx, createdAt, oldGroupCode, -oldSuccess, -oldServerErr, -oldClientAbort); err != nil {
			return err
		}
		return applyServiceStatusBucketStatDelta(ctx, createdAt, newGroupCode, newSuccess, newServerErr, newClientAbort)
	}
	return applyServiceStatusBucketStatDelta(ctx, createdAt, newGroupCode, newSuccess-oldSuccess, newServerErr-oldServerErr, newClientAbort-oldClientAbort)
}

type ServiceStatusBucketStatRow struct {
	GroupCode     string `gorm:"column:group_code"`
	BucketStart   int64  `gorm:"column:bucket_start"`
	Success       int64  `gorm:"column:success_count"`
	ServerErrors  int64  `gorm:"column:server_error_count"`
	ClientAborts  int64  `gorm:"column:client_abort_count"`
	BucketSeconds int64  `gorm:"column:bucket_seconds"`
}

func ListServiceStatusBucketStats(ctx context.Context, start int64, end int64, bucketSeconds int64, groupCodes []string) ([]ServiceStatusBucketStatRow, error) {
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

	q := LOG_DB.WithContext(ctx).Model(&ServiceStatusBucketStat{}).
		Select("group_code, bucket_start, bucket_seconds, success_count, server_error_count, client_abort_count").
		Where("bucket_seconds = ?", bucketSeconds).
		Where("bucket_start >= ? AND bucket_start < ?", start, end)
	if len(groupCodes) > 0 {
		q = q.Where("group_code IN ?", groupCodes)
	}

	rows := make([]ServiceStatusBucketStatRow, 0)
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type serviceStatusBackfillJob struct {
	mu      sync.Mutex
	running bool
	start   int64
	end     int64
}

type ServiceStatusBucketStatsReadiness struct {
	BackfilledStart int64 `json:"backfilled_start"`
	Pending         bool  `json:"pending"`
}

var serviceStatusBackfillJobs sync.Map // bucketSeconds -> *serviceStatusBackfillJob

func RequestServiceStatusBucketStatsBackfill(startBucket int64, endBucketExclusive int64, bucketSeconds int64) {
	if startBucket <= 0 || endBucketExclusive <= 0 || startBucket >= endBucketExclusive {
		return
	}
	switch bucketSeconds {
	case serviceStatusBucketSecondsMinute, serviceStatusBucketSecondsHour, serviceStatusBucketSecondsDay:
	default:
		return
	}

	configHash := getServiceStatusStatsConfigHash()
	meta, err := getServiceStatusBucketStatsMeta(context.Background(), bucketSeconds)
	if err == nil && meta == nil {
		_ = upsertServiceStatusBucketStatsMeta(context.Background(), bucketSeconds, 0, configHash)
	}

	jobAny, _ := serviceStatusBackfillJobs.LoadOrStore(bucketSeconds, &serviceStatusBackfillJob{})
	job := jobAny.(*serviceStatusBackfillJob)

	job.mu.Lock()
	if job.start == 0 || startBucket < job.start {
		job.start = startBucket
	}
	if endBucketExclusive > job.end {
		job.end = endBucketExclusive
	}
	if job.running {
		job.mu.Unlock()
		return
	}
	job.running = true
	job.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				common.SysLog(fmt.Sprintf("service status backfill panic: %v\n%s", r, debug.Stack()))
			}
			job.mu.Lock()
			job.running = false
			job.mu.Unlock()
		}()

		for {
			job.mu.Lock()
			s := job.start
			e := job.end
			job.start = 0
			job.end = 0
			job.mu.Unlock()

			if s <= 0 || e <= 0 || s >= e {
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			err := EnsureServiceStatusBucketStatsBackfilled(ctx, s, e, bucketSeconds)
			cancel()
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					return
				}
				common.SysLog(fmt.Sprintf("service status backfill failed: bucket=%d start=%d end=%d err=%v", bucketSeconds, s, e, err))
				return
			}

			job.mu.Lock()
			hasPending := job.start > 0 && job.end > 0 && job.start < job.end
			job.mu.Unlock()
			if !hasPending {
				return
			}
		}
	}()
}

var serviceStatusBackfillMu sync.Map // bucketSeconds -> *sync.Mutex

func serviceStatusGetBackfillMutex(bucketSeconds int64) *sync.Mutex {
	actual, _ := serviceStatusBackfillMu.LoadOrStore(bucketSeconds, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func PrepareServiceStatusBucketStatsForRead(ctx context.Context, startBucket int64, endBucketExclusive int64, bucketSeconds int64) (ServiceStatusBucketStatsReadiness, error) {
	readiness := ServiceStatusBucketStatsReadiness{}
	if startBucket <= 0 || endBucketExclusive <= 0 || startBucket >= endBucketExclusive {
		return readiness, errors.New("start/end 无效")
	}
	switch bucketSeconds {
	case serviceStatusBucketSecondsMinute, serviceStatusBucketSecondsHour, serviceStatusBucketSecondsDay:
	default:
		return readiness, errors.New("bucketSeconds 无效")
	}

	mu := serviceStatusGetBackfillMutex(bucketSeconds)
	mu.Lock()
	defer mu.Unlock()

	currentConfigHash := strings.TrimSpace(getServiceStatusStatsConfigHash())
	meta, err := getServiceStatusBucketStatsMeta(ctx, bucketSeconds)
	if err != nil {
		return readiness, err
	}

	shouldReset := false
	switch {
	case meta == nil:
		shouldReset = true
		meta = &ServiceStatusBucketStatsMeta{
			BucketSeconds: bucketSeconds,
		}
	case strings.TrimSpace(meta.UAFilterHash) != currentConfigHash:
		shouldReset = true
	}

	if shouldReset {
		if err := resetServiceStatusDerivedCaches(ctx, bucketSeconds); err != nil {
			return readiness, err
		}
		meta.UAFilterHash = currentConfigHash
		meta.BackfilledStart = endBucketExclusive
		if err := upsertServiceStatusBucketStatsMeta(ctx, bucketSeconds, meta.BackfilledStart, meta.UAFilterHash); err != nil {
			return readiness, err
		}
	} else if meta.BackfilledStart == 0 {
		meta.BackfilledStart = endBucketExclusive
		if err := upsertServiceStatusBucketStatsMeta(ctx, bucketSeconds, meta.BackfilledStart, strings.TrimSpace(meta.UAFilterHash)); err != nil {
			return readiness, err
		}
	}

	readiness.BackfilledStart = meta.BackfilledStart
	readiness.Pending = startBucket < meta.BackfilledStart
	return readiness, nil
}

func resetServiceStatusDerivedCaches(ctx context.Context, bucketSeconds int64) error {
	serviceStatusBucketStatFlushMu.Lock()
	defer serviceStatusBucketStatFlushMu.Unlock()

	serviceStatusBucketStatDeltas.mu.Lock()
	if serviceStatusBucketStatDeltas.deltas != nil {
		serviceStatusBucketStatDeltas.deltas = make(map[serviceStatusBucketStatDeltaKey]serviceStatusBucketStatDelta)
	}
	serviceStatusBucketStatDeltas.mu.Unlock()

	if err := LOG_DB.WithContext(ctx).Where("bucket_seconds = ?", bucketSeconds).Delete(&ServiceStatusBucketStat{}).Error; err != nil {
		return err
	}
	return LOG_DB.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ServiceStatusRequestState{}).Error
}

func getServiceStatusBucketStatsMeta(ctx context.Context, bucketSeconds int64) (*ServiceStatusBucketStatsMeta, error) {
	var meta ServiceStatusBucketStatsMeta
	if err := LOG_DB.WithContext(ctx).Where("bucket_seconds = ?", bucketSeconds).First(&meta).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &meta, nil
}

func upsertServiceStatusBucketStatsMeta(ctx context.Context, bucketSeconds int64, backfilledStart int64, uaFilterHash string) error {
	if bucketSeconds <= 0 || backfilledStart < 0 {
		return errors.New("bucketSeconds/backfilledStart 无效")
	}
	meta := ServiceStatusBucketStatsMeta{
		BucketSeconds:   bucketSeconds,
		BackfilledStart: backfilledStart,
		UAFilterHash:    strings.TrimSpace(uaFilterHash),
	}
	return LOG_DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bucket_seconds"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"backfilled_start": backfilledStart, "ua_filter_hash": meta.UAFilterHash, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}),
	}).Create(&meta).Error
}

// EnsureServiceStatusBucketStatsBackfilled backfills bucket stats from historical logs.
//
// It only extends backwards (earlier start) to avoid repeated heavy scans.
// New logs are aggregated incrementally via UpdateServiceStatusBucketStatsOnLogInsert/OnLogTypeChange.
func EnsureServiceStatusBucketStatsBackfilled(ctx context.Context, startBucket int64, endBucketExclusive int64, bucketSeconds int64) error {
	if startBucket <= 0 || endBucketExclusive <= 0 || startBucket >= endBucketExclusive {
		return errors.New("start/end 无效")
	}
	switch bucketSeconds {
	case serviceStatusBucketSecondsMinute, serviceStatusBucketSecondsHour, serviceStatusBucketSecondsDay:
	default:
		return errors.New("bucketSeconds 无效")
	}

	mu := serviceStatusGetBackfillMutex(bucketSeconds)
	mu.Lock()
	defer mu.Unlock()

	currentConfigHash := getServiceStatusStatsConfigHash()

	meta, err := getServiceStatusBucketStatsMeta(ctx, bucketSeconds)
	if err != nil {
		return err
	}
	if meta == nil {
		if err := resetServiceStatusDerivedCaches(ctx, bucketSeconds); err != nil {
			return err
		}
		meta = &ServiceStatusBucketStatsMeta{
			BucketSeconds:   bucketSeconds,
			BackfilledStart: endBucketExclusive,
			UAFilterHash:    strings.TrimSpace(currentConfigHash),
		}
		if err := upsertServiceStatusBucketStatsMeta(ctx, bucketSeconds, meta.BackfilledStart, meta.UAFilterHash); err != nil {
			return err
		}
	}

	storedConfigHash := strings.TrimSpace(meta.UAFilterHash)
	if storedConfigHash != strings.TrimSpace(currentConfigHash) {
		if err := resetServiceStatusDerivedCaches(ctx, bucketSeconds); err != nil {
			return err
		}
		meta.UAFilterHash = strings.TrimSpace(currentConfigHash)
		meta.BackfilledStart = endBucketExclusive
		if err := upsertServiceStatusBucketStatsMeta(ctx, bucketSeconds, meta.BackfilledStart, meta.UAFilterHash); err != nil {
			return err
		}
	}
	if meta.BackfilledStart == 0 {
		meta.BackfilledStart = endBucketExclusive
		if err := upsertServiceStatusBucketStatsMeta(ctx, bucketSeconds, meta.BackfilledStart, meta.UAFilterHash); err != nil {
			return err
		}
	}

	chunkSeconds := serviceStatusBackfillChunkSeconds(bucketSeconds)
	if chunkSeconds < bucketSeconds {
		chunkSeconds = bucketSeconds
	}

	for startBucket < meta.BackfilledStart {
		if err := ctx.Err(); err != nil {
			return err
		}

		chunkEnd := meta.BackfilledStart
		chunkStart := chunkEnd - chunkSeconds
		if chunkStart < startBucket {
			chunkStart = startBucket
		}
		if chunkStart >= chunkEnd {
			break
		}

		if err := BackfillServiceStatusBucketStats(ctx, chunkStart, chunkEnd, bucketSeconds); err != nil {
			return err
		}
		meta.BackfilledStart = chunkStart
		if err := upsertServiceStatusBucketStatsMeta(ctx, bucketSeconds, meta.BackfilledStart, meta.UAFilterHash); err != nil {
			return err
		}
	}
	return nil
}

// BackfillServiceStatusBucketStats scans logs and upserts exact totals into service_status_bucket_stats.
// The range is [start, end).
func BackfillServiceStatusBucketStats(ctx context.Context, start int64, end int64, bucketSeconds int64) error {
	if start <= 0 || end <= 0 || start >= end {
		return errors.New("start/end 无效")
	}
	if bucketSeconds <= 0 {
		return errors.New("bucketSeconds 无效")
	}

	aggRows, err := ListServiceStatusAgg(ctx, start, end, bucketSeconds, nil)
	if err != nil {
		return err
	}

	rows := make([]ServiceStatusBucketStat, 0, len(aggRows))
	for _, row := range aggRows {
		gc := strings.TrimSpace(row.GroupCode)
		if gc == "" || row.BucketStart <= 0 {
			continue
		}
		r := ServiceStatusBucketStat{
			BucketSeconds:    bucketSeconds,
			BucketStart:      row.BucketStart,
			GroupCode:        gc,
			SuccessCount:     row.Success,
			ServerErrorCount: row.ServerErrors,
			ClientAbortCount: row.ClientAborts,
			CreatedAt:        time.Time{},
			UpdatedAt:        time.Time{},
		}
		if r.SuccessCount == 0 && r.ServerErrorCount == 0 && r.ClientAbortCount == 0 {
			continue
		}
		rows = append(rows, r)
	}

	serviceStatusBucketStatFlushMu.Lock()
	defer serviceStatusBucketStatFlushMu.Unlock()

	if err := LOG_DB.WithContext(ctx).
		Where("bucket_seconds = ? AND bucket_start >= ? AND bucket_start < ?", bucketSeconds, start, end).
		Delete(&ServiceStatusBucketStat{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return upsertServiceStatusBucketStatExactRows(ctx, rows)
}

func serviceStatusBackfillChunkSeconds(bucketSeconds int64) int64 {
	switch bucketSeconds {
	case serviceStatusBucketSecondsDay:
		return serviceStatusBucketSecondsDay
	case serviceStatusBucketSecondsHour:
		return 24 * serviceStatusBucketSecondsHour
	case serviceStatusBucketSecondsMinute:
		return 30 * serviceStatusBucketSecondsMinute
	default:
		return bucketSeconds
	}
}
