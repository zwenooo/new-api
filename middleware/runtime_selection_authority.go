package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"one-api/common"
	"one-api/constant"
	"one-api/model"
	"one-api/setting"
	"one-api/types"

	"github.com/gin-gonic/gin"
)

// RuntimeSelectionAuthority is the shared runtime contract for request-group ordering,
// user-agent gating, and direct/compat channel selection across distributor, responses-ws,
// and relay billing candidate resolution.
type RuntimeSelectionAuthority struct {
	RequestPath       string
	RequestedModel    string
	CurrentGroupID    int
	CandidateGroupIDs []int
	IsMessagesRequest bool
	UserAgent         string

	allowUserAgentFn      func(int, string) bool
	groupAllowsModelFn    func(int, string) bool
	directChannelLookupFn func(*gin.Context, int, string) (*model.Channel, error)
	compatChannelLookupFn func(*gin.Context, int, string) (*model.Channel, error)
}

func newRuntimeSelectionAuthority(requestPath string, requestedModel string, currentGroupID int, candidateGroupIDs []int, userAgent string) *RuntimeSelectionAuthority {
	return &RuntimeSelectionAuthority{
		RequestPath:       strings.TrimSpace(requestPath),
		RequestedModel:    normalizeRequestedModelName(requestedModel),
		CurrentGroupID:    currentGroupID,
		CandidateGroupIDs: rebaseGroupCandidatesFromCurrent(candidateGroupIDs, currentGroupID),
		IsMessagesRequest: strings.HasPrefix(strings.TrimSpace(requestPath), "/v1/messages"),
		UserAgent:         strings.TrimSpace(userAgent),
		allowUserAgentFn: func(groupID int, ua string) bool {
			return model.GroupAllowsUserAgent(groupID, ua)
		},
		groupAllowsModelFn: func(groupID int, requestedModel string) bool {
			return model.GroupAllowsModel(groupID, requestedModel)
		},
		directChannelLookupFn: func(c *gin.Context, groupID int, requestedModel string) (*model.Channel, error) {
			return model.CacheGetRandomSatisfiedChannel(c, groupID, requestedModel, 0)
		},
		compatChannelLookupFn: func(c *gin.Context, groupID int, requestedModel string) (*model.Channel, error) {
			return model.CacheGetRandomSatisfiedMessagesToResponsesCompatChannel(c, groupID, requestedModel, 0)
		},
	}
}

func (a *RuntimeSelectionAuthority) matches(requestPath string, requestedModel string, requestGroupID int) bool {
	if a == nil {
		return false
	}
	if a.RequestPath != strings.TrimSpace(requestPath) {
		return false
	}
	if a.RequestedModel != normalizeRequestedModelName(requestedModel) {
		return false
	}
	if requestGroupID > 0 && requestGroupID != a.CurrentGroupID {
		return false
	}
	return true
}

func (a *RuntimeSelectionAuthority) ApplyContext(c *gin.Context) {
	if c == nil || a == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroupId, a.CurrentGroupID)
	common.SetContextKey(c, constant.ContextKeyResolvedGroupCandidateIds, append([]int(nil), a.CandidateGroupIDs...))
	common.SetContextKey(c, constant.ContextKeyRuntimeSelectionAuthority, a)
}

func (a *RuntimeSelectionAuthority) SetSelectedGroup(groupID int) {
	if a == nil || groupID <= 0 {
		return
	}
	a.CurrentGroupID = groupID
	a.CandidateGroupIDs = rebaseGroupCandidatesFromCurrent(a.CandidateGroupIDs, groupID)
}

func (a *RuntimeSelectionAuthority) AllowsUserAgent(groupID int) bool {
	if a == nil || groupID <= 0 || a.allowUserAgentFn == nil {
		return false
	}
	return a.allowUserAgentFn(groupID, a.UserAgent)
}

func (a *RuntimeSelectionAuthority) DisplayGroupSummary() string {
	if a == nil {
		return "未知分组"
	}
	return formatRuntimeSelectionGroupSummary(a.CandidateGroupIDs, a.CurrentGroupID)
}

func (a *RuntimeSelectionAuthority) lookupStepsForGroup(c *gin.Context, groupID int) []groupChannelSelectionStep {
	if a == nil || groupID <= 0 {
		return nil
	}
	steps := []groupChannelSelectionStep{
		{
			kind: "direct",
			lookup: func() (*model.Channel, string, error) {
				channel, err := a.directChannelLookupFn(c, groupID, a.RequestedModel)
				return channel, a.RequestedModel, err
			},
		},
	}
	if !a.IsMessagesRequest || a.groupAllowsModelFn == nil || !a.groupAllowsModelFn(groupID, a.RequestedModel) {
		return steps
	}
	if a.compatChannelLookupFn == nil {
		return steps
	}
	return append(steps, groupChannelSelectionStep{
		kind: "messages_to_responses_compat",
		lookup: func() (*model.Channel, string, error) {
			channel, err := a.compatChannelLookupFn(c, groupID, a.RequestedModel)
			return channel, a.RequestedModel, err
		},
	})
}

func (a *RuntimeSelectionAuthority) LookupChannelForGroup(c *gin.Context, groupID int) (*model.Channel, string, string, error) {
	if a == nil {
		return nil, "", "", errors.New("runtime selection authority is nil")
	}
	var deferredErr *groupChannelSelectionError
	for _, step := range a.lookupStepsForGroup(c, groupID) {
		if step.lookup == nil {
			continue
		}
		channel, lookupModel, err := step.lookup()
		if err != nil {
			if model.IsChannelConcurrencyLimitReachedErr(err) {
				if deferredErr == nil {
					deferredErr = &groupChannelSelectionError{
						GroupID:     groupID,
						Step:        step.kind,
						LookupModel: lookupModel,
						Err:         err,
					}
				}
				continue
			}
			return nil, lookupModel, step.kind, err
		}
		if channel != nil {
			return channel, lookupModel, step.kind, nil
		}
	}
	if deferredErr != nil {
		return nil, deferredErr.LookupModel, deferredErr.Step, deferredErr.Err
	}
	return nil, a.RequestedModel, "", nil
}

func (a *RuntimeSelectionAuthority) SelectChannel(c *gin.Context) (*model.Channel, int, bool, *groupChannelSelectionError) {
	if a == nil {
		return nil, 0, false, nil
	}
	return selectChannelByCandidateGroupOrder(
		a.CandidateGroupIDs,
		func(groupID int) bool {
			return a.AllowsUserAgent(groupID)
		},
		func(groupID int) []groupChannelSelectionStep {
			return a.lookupStepsForGroup(c, groupID)
		},
	)
}

func formatRuntimeSelectionGroupLabel(groupID int) string {
	if groupID <= 0 {
		return "未知分组"
	}
	if label, ok := model.GetGroupLabelByID(groupID); ok {
		return label
	}
	return "未知分组"
}

func FormatGroupLabelForRuntimeSelection(groupID int) string {
	return formatRuntimeSelectionGroupLabel(groupID)
}

func formatRuntimeSelectionGroupSummary(candidateGroupIDs []int, fallbackGroupID int) string {
	if len(candidateGroupIDs) == 0 {
		return formatRuntimeSelectionGroupLabel(fallbackGroupID)
	}
	if len(candidateGroupIDs) == 1 {
		return formatRuntimeSelectionGroupLabel(candidateGroupIDs[0])
	}
	parts := make([]string, 0, len(candidateGroupIDs))
	for _, groupID := range candidateGroupIDs {
		if groupID <= 0 {
			continue
		}
		parts = append(parts, formatRuntimeSelectionGroupLabel(groupID))
	}
	if len(parts) == 0 {
		return formatRuntimeSelectionGroupLabel(fallbackGroupID)
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ","))
}

func ResolveRuntimeSelectionAuthority(c *gin.Context, requestPath string, requestedModel string, requestGroupID int) (*RuntimeSelectionAuthority, *types.NewAPIError) {
	if c == nil {
		return nil, types.NewErrorWithStatusCode(errors.New("context is nil"), types.ErrorCodeInvalidRequest, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	requestPath = strings.TrimSpace(requestPath)
	requestedModel = normalizeRequestedModelName(requestedModel)
	if authority, ok := common.GetContextKeyType[*RuntimeSelectionAuthority](c, constant.ContextKeyRuntimeSelectionAuthority); ok && authority != nil && authority.matches(requestPath, requestedModel, requestGroupID) {
		return authority, nil
	}

	usingGroupID := common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
	if usingGroupID <= 0 {
		return nil, types.NewErrorWithStatusCode(errors.New("无效的分组"), types.ErrorCodeAccessDenied, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	tokenAllowedGroupIDs, _ := common.GetContextKeyType[[]int](c, constant.ContextKeyTokenAllowedGroupIds)
	candidateGroupIDs := []int{usingGroupID}

	if strings.HasPrefix(requestPath, "/pg/chat/completions") && requestGroupID > 0 {
		if !setting.GroupInUserUsableGroups(requestGroupID) && requestGroupID != usingGroupID {
			return nil, types.NewErrorWithStatusCode(errors.New("无权访问该分组"), types.ErrorCodeAccessDenied, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		usingGroupID = requestGroupID
		candidateGroupIDs = []int{usingGroupID}
	} else if len(tokenAllowedGroupIDs) > 0 && requestedModel != "" {
		resolvedGroupIDs, resolveErr := resolveUsingGroupCandidatesForModelRequest(
			requestPath,
			usingGroupID,
			tokenAllowedGroupIDs,
			common.GetContextKeyInt(c, constant.ContextKeyUserId),
			requestedModel,
		)
		if resolveErr != nil {
			return nil, types.NewErrorWithStatusCode(resolveErr, types.ErrorCodeGetChannelFailed, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
		}
		candidateGroupIDs = resolvedGroupIDs
		if len(candidateGroupIDs) > 0 && candidateGroupIDs[0] > 0 {
			usingGroupID = candidateGroupIDs[0]
		}
	}

	authority := newRuntimeSelectionAuthority(requestPath, requestedModel, usingGroupID, candidateGroupIDs, c.GetHeader("User-Agent"))
	authority.ApplyContext(c)
	return authority, nil
}
