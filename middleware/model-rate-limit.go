package middleware

import (
	"context"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/common/limiter"
	"one-api/constant"
	"one-api/setting"
	"one-api/types"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
)

// ModelRequestRateLimitGuard holds the context required to record a successful request
// after rate-limit checks passed.
type ModelRequestRateLimitGuard struct {
	enabled         bool
	redisEnabled    bool
	durationSeconds int64

	userID          int
	totalMaxCount   int
	successMaxCount int

	successKey string
	successMem string
}

func (g *ModelRequestRateLimitGuard) RecordSuccess() {
	if g == nil || !g.enabled {
		return
	}
	if g.successMaxCount <= 0 {
		return
	}

	if g.redisEnabled {
		ctx := context.Background()
		rdb := common.RDB
		if rdb == nil {
			return
		}
		recordRedisRequest(ctx, rdb, g.successKey, g.successMaxCount)
		return
	}

	// Memory limiter: record success at the end.
	inMemoryRateLimiter.Request(g.successMem, g.successMaxCount, g.durationSeconds)
}

// AcquireModelRequestRateLimitGuard checks current rate-limit settings and returns a guard
// that should be used to record success when the request completes successfully.
//
// Note: This function only performs "check + record total" at acquire-time. Success counting
// is recorded explicitly via the returned guard.
func AcquireModelRequestRateLimitGuard(c *gin.Context, groupID int) (*ModelRequestRateLimitGuard, *types.NewAPIError) {
	if !setting.ModelRequestRateLimitEnabled {
		return &ModelRequestRateLimitGuard{enabled: false}, nil
	}
	if c == nil {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("missing request context"), types.ErrorCode("rate_limit_check_failed"), http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}

	userID := c.GetInt("id")
	if userID <= 0 {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("missing user id for rate limit"), types.ErrorCode("rate_limit_check_failed"), http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	userIDStr := strconv.Itoa(userID)

	durationSeconds := int64(setting.ModelRequestRateLimitDurationMinutes * 60)
	totalMaxCount := setting.ModelRequestRateLimitCount
	successMaxCount := setting.ModelRequestRateLimitSuccessCount

	// Apply group overrides (groupID is expected to be the final using_group_id for this request).
	if groupID > 0 {
		if groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(groupID); found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
		}
	}

	guard := &ModelRequestRateLimitGuard{
		enabled:         true,
		redisEnabled:    common.RedisEnabled,
		durationSeconds: durationSeconds,
		userID:          userID,
		totalMaxCount:   totalMaxCount,
		successMaxCount: successMaxCount,
	}

	if common.RedisEnabled {
		ctx := context.Background()
		rdb := common.RDB
		if rdb == nil {
			return nil, types.NewErrorWithStatusCode(fmt.Errorf("redis is enabled but client is nil"), types.ErrorCode("rate_limit_check_failed"), http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
		}

		// 1) Check success request limit (recorded on success)
		guard.successKey = fmt.Sprintf("rateLimit:%s:%s:%d", ModelRequestRateLimitSuccessCountMark, userIDStr, groupID)
		allowed, err := checkRedisRateLimit(ctx, rdb, guard.successKey, successMaxCount, durationSeconds)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(fmt.Errorf("检查成功请求数限制失败: %w", err), types.ErrorCode("rate_limit_check_failed"), http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
		}
		if !allowed {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, successMaxCount),
				types.ErrorCode(""),
				http.StatusTooManyRequests,
				types.ErrOptionWithSkipRetry(),
			)
		}

		// 2) Check total request limit (includes failures): token-bucket limiter
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s:%s:%d", ModelRequestRateLimitCountMark, userIDStr, groupID)
			tb := limiter.New(ctx, rdb)
			allowed, err = tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(int64(totalMaxCount)*durationSeconds),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(durationSeconds),
			)
			if err != nil {
				return nil, types.NewErrorWithStatusCode(fmt.Errorf("检查总请求数限制失败: %w", err), types.ErrorCode("rate_limit_check_failed"), http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
			}
			if !allowed {
				return nil, types.NewErrorWithStatusCode(
					fmt.Errorf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, totalMaxCount),
					types.ErrorCode(""),
					http.StatusTooManyRequests,
					types.ErrOptionWithSkipRetry(),
				)
			}
		}
		return guard, nil
	}

	// Memory limiter
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	// 1) Total request limit (recorded at acquire-time)
	if totalMaxCount > 0 {
		totalKey := fmt.Sprintf("%s%s:%d", ModelRequestRateLimitCountMark, userIDStr, groupID)
		if !inMemoryRateLimiter.Request(totalKey, totalMaxCount, durationSeconds) {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, totalMaxCount),
				types.ErrorCode(""),
				http.StatusTooManyRequests,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}

	// 2) Success request limit: check with a temporary key at acquire-time
	// so that parallel in-flight requests can't easily overshoot. Record success at the end.
	successKey := fmt.Sprintf("%s%s:%d", ModelRequestRateLimitSuccessCountMark, userIDStr, groupID)
	checkKey := successKey + "_check"
	if !inMemoryRateLimiter.Request(checkKey, successMaxCount, durationSeconds) {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, successMaxCount),
			types.ErrorCode(""),
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
		)
	}
	guard.successMem = successKey
	return guard, nil
}

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
	oldTime, err := time.Parse(timeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	nowTimeStr := time.Now().Format(timeFormat)
	nowTime, err := time.Parse(timeFormat, nowTimeStr)
	if err != nil {
		return false, err
	}
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
		return false, nil
	}

	return true, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int) {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return
	}

	now := time.Now().Format(timeFormat)
	rdb.LPush(ctx, key, now)
	rdb.LTrim(ctx, key, 0, int64(maxCount-1))
	rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
}

// Redis限流处理器
func redisRateLimitHandler(duration int64, groupID int, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		ctx := context.Background()
		rdb := common.RDB

		// 1. 检查成功请求数限制
		successKey := fmt.Sprintf("rateLimit:%s:%s:%d", ModelRequestRateLimitSuccessCountMark, userId, groupID)
		allowed, err := checkRedisRateLimit(ctx, rdb, successKey, successMaxCount, duration)
		if err != nil {
			fmt.Println("检查成功请求数限制失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, successMaxCount))
			return
		}

		//2.检查总请求数限制并记录总请求（当totalMaxCount为0时会自动跳过，使用令牌桶限流器
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s:%s:%d", ModelRequestRateLimitCountMark, userId, groupID)
			// 初始化
			tb := limiter.New(ctx, rdb)
			allowed, err = tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(int64(totalMaxCount)*duration),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(duration),
			)

			if err != nil {
				fmt.Println("检查总请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}

			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, totalMaxCount))
			}
		}

		// 4. 处理请求
		c.Next()

		// 5. 如果请求成功，记录成功请求
		if c.Writer.Status() < 400 {
			recordRedisRequest(ctx, rdb, successKey, successMaxCount)
		}
	}
}

// 内存限流处理器
func memoryRateLimitHandler(duration int64, groupID int, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		totalKey := fmt.Sprintf("%s%s:%d", ModelRequestRateLimitCountMark, userId, groupID)
		successKey := fmt.Sprintf("%s%s:%d", ModelRequestRateLimitSuccessCountMark, userId, groupID)

		// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 2. 检查成功请求数限制
		// 使用一个临时key来检查限制，这样可以避免实际记录
		checkKey := successKey + "_check"
		if !inMemoryRateLimiter.Request(checkKey, successMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 3. 处理请求
		c.Next()

		// 4. 如果请求成功，记录到实际的成功请求计数中
		if c.Writer.Status() < 400 {
			inMemoryRateLimiter.Request(successKey, successMaxCount, duration)
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		// 计算限流参数
		duration := int64(setting.ModelRequestRateLimitDurationMinutes * 60)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount

		// 获取分组（以本次实际使用的分组为准）
		groupID := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
		if groupID <= 0 {
			groupID = common.GetContextKeyInt(c, constant.ContextKeyDefaultModelGroupId)
		}
		if groupID > 0 {
			// 获取分组的限流配置
			groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(groupID)
			if found {
				totalMaxCount = groupTotalCount
				successMaxCount = groupSuccessCount
			}
		}

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(duration, groupID, totalMaxCount, successMaxCount)(c)
		} else {
			memoryRateLimitHandler(duration, groupID, totalMaxCount, successMaxCount)(c)
		}
	}
}
