package model

import (
	"net/http/httptest"
	"testing"

	"one-api/common"
	"one-api/constant"

	"github.com/gin-gonic/gin"
)

func TestUserBaseWriteContextSeedsUsingGroupIDWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	user := &UserBase{
		GroupId:         12,
		UserGroupId:     34,
		Quota:           100,
		Status:          common.UserStatusEnabled,
		Username:        "cache-user",
		BaseMultiplier:  1.2,
		PlanType:        "default",
		PlanStartAt:     10,
		PlanExpireAt:    20,
		DailyQuotaLimit: 50,
		DailyQuotaUsed:  5,
	}

	user.WriteContext(ctx)

	if got := common.GetContextKeyInt(ctx, constant.ContextKeyUsingGroupId); got != 12 {
		t.Fatalf("using_group_id = %d, want 12", got)
	}
	if got := common.GetContextKeyInt(ctx, constant.ContextKeyUserGroupId); got != 34 {
		t.Fatalf("user_group_id = %d, want 34", got)
	}
	if got := common.GetContextKeyInt(ctx, constant.ContextKeyDefaultModelGroupId); got != 12 {
		t.Fatalf("default_model_group_id = %d, want 12", got)
	}
}

func TestUserBaseWriteContextPreservesExistingUsingGroupID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroupId, 99)

	user := &UserBase{
		GroupId:        12,
		UserGroupId:    34,
		Status:         common.UserStatusEnabled,
		Username:       "cache-user",
		BaseMultiplier: 1.2,
	}

	user.WriteContext(ctx)

	if got := common.GetContextKeyInt(ctx, constant.ContextKeyUsingGroupId); got != 99 {
		t.Fatalf("using_group_id = %d, want 99", got)
	}
	if got := common.GetContextKeyInt(ctx, constant.ContextKeyUserGroupId); got != 34 {
		t.Fatalf("user_group_id = %d, want 34", got)
	}
	if got := common.GetContextKeyInt(ctx, constant.ContextKeyDefaultModelGroupId); got != 12 {
		t.Fatalf("default_model_group_id = %d, want 12", got)
	}
}
