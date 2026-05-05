package service

import (
	"fmt"
	"one-api/common"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/setting/operation_setting"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type builtInToolChargeSummary struct {
	ResponsesWebSearchCallCount    int
	ResponsesWebSearchContextSize  string
	ResponsesWebSearchPrice        float64
	ResponsesWebSearchQuota        float64
	ResponsesWebSearchVisibleQuota float64
	ResponsesWebSearchCostQuota    float64
	ClaudeWebSearchCallCount       int
	ClaudeWebSearchPrice           float64
	ClaudeWebSearchQuota           float64
	ClaudeWebSearchVisibleQuota    float64
	ClaudeWebSearchCostQuota       float64
	FileSearchCallCount            int
	FileSearchPrice                float64
	FileSearchQuota                float64
	FileSearchVisibleQuota         float64
	FileSearchCostQuota            float64
}

func calculateBuiltInToolChargeSummary(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string, upstreamModelName string, groupRatio float64, publicGroupRatio float64) builtInToolChargeSummary {
	summary := builtInToolChargeSummary{}
	responsesPricingModelName := strings.TrimSpace(upstreamModelName)
	if responsesPricingModelName == "" {
		responsesPricingModelName = strings.TrimSpace(modelName)
	}

	if relayInfo != nil && relayInfo.ResponsesUsageInfo != nil {
		webSearchTool := relaycommon.EnsureResponsesBuiltInTool(relayInfo, dto.BuildInToolWebSearch)
		if webSearchTool != nil && webSearchTool.CallCount > 0 {
			searchContextSize := strings.TrimSpace(webSearchTool.SearchContextSize)
			if searchContextSize == "" {
				searchContextSize = "medium"
			}
			summary.ResponsesWebSearchCallCount = webSearchTool.CallCount
			summary.ResponsesWebSearchContextSize = searchContextSize
			summary.ResponsesWebSearchPrice = operation_setting.GetWebSearchPricePerThousand(responsesPricingModelName, searchContextSize)
			summary.ResponsesWebSearchQuota = summary.ResponsesWebSearchPrice * float64(webSearchTool.CallCount) / 1000 * groupRatio * common.QuotaPerUnit
			summary.ResponsesWebSearchVisibleQuota = summary.ResponsesWebSearchPrice * float64(webSearchTool.CallCount) / 1000 * publicGroupRatio * common.QuotaPerUnit
			costWebSearchPrice := operation_setting.GetWebSearchPricePerThousand(responsesPricingModelName, searchContextSize)
			summary.ResponsesWebSearchCostQuota = costWebSearchPrice * float64(webSearchTool.CallCount) / 1000 * common.QuotaPerUnit
		}

		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists && fileSearchTool != nil && fileSearchTool.CallCount > 0 {
			summary.FileSearchCallCount = fileSearchTool.CallCount
			summary.FileSearchPrice = operation_setting.GetFileSearchPricePerThousand()
			summary.FileSearchQuota = summary.FileSearchPrice * float64(fileSearchTool.CallCount) / 1000 * groupRatio * common.QuotaPerUnit
			summary.FileSearchVisibleQuota = summary.FileSearchPrice * float64(fileSearchTool.CallCount) / 1000 * publicGroupRatio * common.QuotaPerUnit
			summary.FileSearchCostQuota = summary.FileSearchPrice * float64(fileSearchTool.CallCount) / 1000 * common.QuotaPerUnit
		}
	}

	if ctx != nil {
		claudeWebSearchCallCount := ctx.GetInt("claude_web_search_requests")
		if claudeWebSearchCallCount > 0 {
			summary.ClaudeWebSearchCallCount = claudeWebSearchCallCount
			summary.ClaudeWebSearchPrice = operation_setting.GetClaudeWebSearchPricePerThousand()
			summary.ClaudeWebSearchQuota = summary.ClaudeWebSearchPrice * float64(claudeWebSearchCallCount) / 1000 * groupRatio * common.QuotaPerUnit
			summary.ClaudeWebSearchVisibleQuota = summary.ClaudeWebSearchPrice * float64(claudeWebSearchCallCount) / 1000 * publicGroupRatio * common.QuotaPerUnit
			summary.ClaudeWebSearchCostQuota = summary.ClaudeWebSearchPrice * float64(claudeWebSearchCallCount) / 1000 * common.QuotaPerUnit
		}
	}

	return summary
}

func (summary builtInToolChargeSummary) ExtraContent() string {
	parts := make([]string, 0, 3)
	if summary.ResponsesWebSearchCallCount > 0 {
		parts = append(parts, fmt.Sprintf("Web Search 调用 %d 次，上下文大小 %s，调用花费 %s",
			summary.ResponsesWebSearchCallCount,
			summary.ResponsesWebSearchContextSize,
			decimal.NewFromFloat(summary.ResponsesWebSearchQuota).String(),
		))
	}
	if summary.ClaudeWebSearchCallCount > 0 {
		parts = append(parts, fmt.Sprintf("Claude Web Search 调用 %d 次，调用花费 %s",
			summary.ClaudeWebSearchCallCount,
			decimal.NewFromFloat(summary.ClaudeWebSearchQuota).String(),
		))
	}
	if summary.FileSearchCallCount > 0 {
		parts = append(parts, fmt.Sprintf("File Search 调用 %d 次，调用花费 %s",
			summary.FileSearchCallCount,
			decimal.NewFromFloat(summary.FileSearchQuota).String(),
		))
	}
	return strings.Join(parts, ", ")
}
