package service

import (
	"errors"
	"fmt"
	"one-api/billing"
	"one-api/logger"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"strings"
)

type quotaDetailError struct {
	msg string
	err error
}

func (e *quotaDetailError) Error() string {
	return e.msg
}

func (e *quotaDetailError) Unwrap() error {
	return e.err
}

func wrapQuotaDetailError(err error, message string) error {
	if err == nil {
		return nil
	}
	if strings.TrimSpace(message) == "" {
		return err
	}
	var existing *quotaDetailError
	if errors.As(err, &existing) {
		return err
	}
	return &quotaDetailError{msg: message, err: err}
}

func buildQuotaErrorContext(relayInfo *relaycommon.RelayInfo) (bucket string, usingGroupID int) {
	if relayInfo == nil {
		return "", 0
	}
	return relayInfo.QuotaBucket, relayInfo.UsingGroupId
}

func formatQuotaOrDash(quota int) string {
	// -1: unknown, -2: infinity
	if quota == -2 {
		return "∞"
	}
	if quota < 0 {
		return "-"
	}
	return logger.FormatQuota(quota)
}

func formatSubscriptionQuotaOrDash(bucket string, quota int) string {
	// -1: unknown, -2: infinity
	if quota == -2 {
		return "∞"
	}
	if quota < 0 {
		return "-"
	}
	switch bucket {
	case model.UserQuotaBucketTokens, model.UserQuotaBucketPayToken:
		return fmt.Sprintf("%s tokens", billing.StoredUnitsToDisplayString(quota))
	case model.UserQuotaBucketRequest, model.UserQuotaBucketPayRequest:
		return fmt.Sprintf("%s requests", billing.StoredUnitsToDisplayString(quota))
	}
	return logger.FormatQuota(quota)
}

func buildUserDailyQuotaExceededMessage(relayInfo *relaycommon.RelayInfo, expectedQuota int, subscriptionRemaining int, subscriptionAvailableToday int) string {
	bucket, groupID := buildQuotaErrorContext(relayInfo)
	if bucket == "" {
		bucket = "-"
	}
	group := "-"
	if label, ok := model.GetGroupLabelByID(groupID); ok {
		group = label
	}

	expected := formatSubscriptionQuotaOrDash(bucket, expectedQuota)
	remaining := formatSubscriptionQuotaOrDash(bucket, subscriptionRemaining)
	availableToday := formatSubscriptionQuotaOrDash(bucket, subscriptionAvailableToday)

	cn := fmt.Sprintf("用户订阅当日额度已用尽（计费桶: %s；当前分组: %s；订阅剩余: %s；今日可用: %s；本次预计: %s）", bucket, group, remaining, availableToday, expected)
	en := fmt.Sprintf("user daily quota exceeded (bucket: %s; group: %s; subscription remaining: %s; available today: %s; expected: %s)", bucket, group, remaining, availableToday, expected)
	return cn + " / " + en
}

func buildTokenDailyQuotaExceededMessage(relayInfo *relaycommon.RelayInfo, expectedQuota int, tokenDailyLimit int, tokenUsedToday int, tokenRemainingToday int) string {
	bucket, groupID := buildQuotaErrorContext(relayInfo)
	if bucket == "" {
		bucket = "-"
	}
	group := "-"
	if label, ok := model.GetGroupLabelByID(groupID); ok {
		group = label
	}

	expected := formatQuotaOrDash(expectedQuota)
	limit := formatQuotaOrDash(tokenDailyLimit)
	used := formatQuotaOrDash(tokenUsedToday)
	remainingToday := formatQuotaOrDash(tokenRemainingToday)

	cn := fmt.Sprintf("令牌当日额度已用尽（计费桶: %s；当前分组: %s；令牌日限: %s；今日已用: %s；今日剩余: %s；本次预计: %s）", bucket, group, limit, used, remainingToday, expected)
	en := fmt.Sprintf("token daily quota exceeded (bucket: %s; group: %s; token daily limit: %s; used today: %s; remaining today: %s; expected: %s)", bucket, group, limit, used, remainingToday, expected)
	return cn + " / " + en
}

func buildSubscriptionQuotaInsufficientMessage(relayInfo *relaycommon.RelayInfo, expectedQuota int, subscriptionRemaining int, subscriptionAvailableToday int) string {
	bucket, groupID := buildQuotaErrorContext(relayInfo)
	if bucket == "" {
		bucket = "-"
	}
	group := "-"
	if label, ok := model.GetGroupLabelByID(groupID); ok {
		group = label
	}

	expected := formatSubscriptionQuotaOrDash(bucket, expectedQuota)
	remaining := formatSubscriptionQuotaOrDash(bucket, subscriptionRemaining)
	availableToday := formatSubscriptionQuotaOrDash(bucket, subscriptionAvailableToday)

	cn := fmt.Sprintf("订阅额度不足（计费桶: %s；当前分组: %s；订阅剩余: %s；今日可用: %s；本次预计: %s）", bucket, group, remaining, availableToday, expected)
	en := fmt.Sprintf("insufficient subscription quota (bucket: %s; group: %s; subscription remaining: %s; available today: %s; expected: %s)", bucket, group, remaining, availableToday, expected)
	return cn + " / " + en
}

func buildFreeQuotaInsufficientMessage(relayInfo *relaycommon.RelayInfo, expectedQuota int, freeRemaining int) string {
	bucket, groupID := buildQuotaErrorContext(relayInfo)
	if bucket == "" {
		bucket = "-"
	}
	group := "-"
	if label, ok := model.GetGroupLabelByID(groupID); ok {
		group = label
	}

	expected := formatQuotaOrDash(expectedQuota)
	remaining := formatQuotaOrDash(freeRemaining)

	cn := fmt.Sprintf("自由额度不足（计费桶: %s；当前分组: %s；自由剩余: %s；本次预计: %s）", bucket, group, remaining, expected)
	en := fmt.Sprintf("insufficient free quota (bucket: %s; group: %s; free remaining: %s; expected: %s)", bucket, group, remaining, expected)
	return cn + " / " + en
}
