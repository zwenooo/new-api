package setting

import (
	"encoding/json"
	"fmt"
	"math"
	"one-api/common"
	"sync"
)

// ModelRequestConcurrencyLimitEnabled controls whether per-user in-flight concurrency limits
// are enforced for streaming requests (SSE/WebSocket) on relay routes.
var ModelRequestConcurrencyLimitEnabled = false

// ModelRequestConcurrencyLimit is the default per-user max in-flight streaming requests.
// 0 means unlimited.
var ModelRequestConcurrencyLimit = 3

// ModelRequestConcurrencyLimitWaitSeconds controls how long to wait for a slot before
// returning 429. 0 means fail fast.
var ModelRequestConcurrencyLimitWaitSeconds = 0

// ModelRequestConcurrencyLimitGroup allows overriding concurrency limit by group_id.
// Format: {"4": 3, "7": 10}. 0 means unlimited.
var ModelRequestConcurrencyLimitGroup = map[int]int{}

var ModelRequestConcurrencyLimitMutex sync.RWMutex

func ModelRequestConcurrencyLimitGroup2JSONString() string {
	ModelRequestConcurrencyLimitMutex.RLock()
	defer ModelRequestConcurrencyLimitMutex.RUnlock()

	jsonBytes, err := json.Marshal(ModelRequestConcurrencyLimitGroup)
	if err != nil {
		common.SysLog("error marshalling model concurrency limit group: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestConcurrencyLimitGroupByJSONString(jsonStr string) error {
	ModelRequestConcurrencyLimitMutex.Lock()
	defer ModelRequestConcurrencyLimitMutex.Unlock()

	next := make(map[int]int)
	if err := json.Unmarshal([]byte(jsonStr), &next); err != nil {
		return err
	}
	for groupID, limit := range next {
		if limit < 0 {
			return fmt.Errorf("group_id %d has negative concurrency limit value: %d", groupID, limit)
		}
		if limit > math.MaxInt32 {
			return fmt.Errorf("group_id %d concurrency limit %d exceeds max 2147483647", groupID, limit)
		}
	}
	ModelRequestConcurrencyLimitGroup = next
	return nil
}

func GetGroupConcurrencyLimit(groupID int) (limit int, found bool) {
	ModelRequestConcurrencyLimitMutex.RLock()
	defer ModelRequestConcurrencyLimitMutex.RUnlock()

	if ModelRequestConcurrencyLimitGroup == nil {
		return 0, false
	}
	limit, found = ModelRequestConcurrencyLimitGroup[groupID]
	return limit, found
}

func CheckModelRequestConcurrencyLimitGroup(jsonStr string) error {
	check := make(map[int]int)
	if err := json.Unmarshal([]byte(jsonStr), &check); err != nil {
		return err
	}
	for groupID, limit := range check {
		if limit < 0 {
			return fmt.Errorf("group_id %d has negative concurrency limit value: %d", groupID, limit)
		}
		if limit > math.MaxInt32 {
			return fmt.Errorf("group_id %d concurrency limit %d exceeds max 2147483647", groupID, limit)
		}
	}
	return nil
}
