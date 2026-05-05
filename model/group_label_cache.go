package model

import (
	"strings"
	"sync"
)

var (
	groupLabelByID   map[int]string
	groupLabelByIDMu sync.RWMutex
)

func setGroupLabelsLocked(labels map[int]string) {
	groupLabelByID = labels
}

func syncGroupLabelsFromGroups(groups []Group) {
	labels := make(map[int]string, len(groups))
	for _, g := range groups {
		if g.Id <= 0 {
			continue
		}
		label := strings.TrimSpace(g.DisplayName)
		if label == "" {
			label = strings.TrimSpace(g.Code)
		}
		if label == "" {
			continue
		}
		labels[g.Id] = label
	}
	groupLabelByIDMu.Lock()
	setGroupLabelsLocked(labels)
	groupLabelByIDMu.Unlock()
}

// GetGroupLabelByID returns the user-facing label for a group_id.
// It is populated by SyncGroupSettingsFromDB and kept in-memory.
func GetGroupLabelByID(groupID int) (string, bool) {
	if groupID <= 0 {
		return "", false
	}
	groupLabelByIDMu.RLock()
	label, ok := groupLabelByID[groupID]
	groupLabelByIDMu.RUnlock()
	label = strings.TrimSpace(label)
	if label == "" {
		return "", false
	}
	return label, ok
}

