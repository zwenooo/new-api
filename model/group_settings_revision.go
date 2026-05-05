package model

import (
	"one-api/common"
	"strconv"
	"strings"
	"sync"
	"time"
)

const groupSettingsRevisionOptionKey = "group_settings.revision"

var (
	groupSettingsRevisionMu          sync.Mutex
	groupSettingsLastSyncedRevision  string
	groupSettingsLastSyncedRevisionT time.Time
)

func readGroupSettingsRevisionFromOptionMap() string {
	common.OptionMapRWMutex.RLock()
	rev := strings.TrimSpace(common.OptionMap[groupSettingsRevisionOptionKey])
	common.OptionMapRWMutex.RUnlock()
	return rev
}

func markGroupSettingsSynced(rev string) {
	groupSettingsRevisionMu.Lock()
	groupSettingsLastSyncedRevision = strings.TrimSpace(rev)
	groupSettingsLastSyncedRevisionT = time.Now()
	groupSettingsRevisionMu.Unlock()
}

func getGroupSettingsSyncState() (string, bool) {
	groupSettingsRevisionMu.Lock()
	rev := groupSettingsLastSyncedRevision
	hasSynced := !groupSettingsLastSyncedRevisionT.IsZero()
	groupSettingsRevisionMu.Unlock()
	return rev, hasSynced
}

func BumpGroupSettingsRevision() error {
	rev := strconv.FormatInt(time.Now().UnixNano(), 10)
	return UpdateOption(groupSettingsRevisionOptionKey, rev)
}

func EnsureGroupSettingsSynced(force bool) error {
	currentRev := readGroupSettingsRevisionFromOptionMap()
	lastRev, hasSynced := getGroupSettingsSyncState()
	if !force {
		if currentRev != "" && currentRev == lastRev {
			return nil
		}
		if currentRev == "" && hasSynced {
			return nil
		}
	}
	common.SysLog("syncing group settings from database")
	return SyncGroupSettingsFromDB(nil)
}

func RefreshGroupSettings() error {
	if err := BumpGroupSettingsRevision(); err != nil {
		return err
	}
	return SyncGroupSettingsFromDB(nil)
}
