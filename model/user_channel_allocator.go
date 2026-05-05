package model

import (
	"hash/crc32"
	"sort"
	"sync"
	"time"
)

type userChannelAssignment struct {
	channelID int
	lastUsed  time.Time
}

type userChannelAllocator struct {
	mu             sync.Mutex
	userToChannel  map[string]userChannelAssignment
	channelUseCount map[int]int
	lastCleanup    time.Time
}

var globalUserChannelAllocator = newUserChannelAllocator()

func newUserChannelAllocator() *userChannelAllocator {
	return &userChannelAllocator{
		userToChannel:   make(map[string]userChannelAssignment),
		channelUseCount: make(map[int]int),
	}
}

func (a *userChannelAllocator) getBaseChannelID(userKey string, candidateIDs []int, mutateAssignment bool, now time.Time, ttl time.Duration) (int, bool) {
	if userKey == "" || len(candidateIDs) == 0 {
		return 0, false
	}

	sort.Ints(candidateIDs)

	a.mu.Lock()
	defer a.mu.Unlock()

	a.cleanupExpiredLocked(now, ttl)

	if entry, ok := a.userToChannel[userKey]; ok {
		if !ttlExpired(entry.lastUsed, now, ttl) {
			entry.lastUsed = now
			a.userToChannel[userKey] = entry
			if containsInt(candidateIDs, entry.channelID) {
				return entry.channelID, true
			}

			if !mutateAssignment {
				return deterministicPickChannel(userKey, candidateIDs), true
			}

			a.decrementChannelUseLocked(entry.channelID)
			delete(a.userToChannel, userKey)
		} else {
			a.decrementChannelUseLocked(entry.channelID)
			delete(a.userToChannel, userKey)
		}
	}

	if !mutateAssignment {
		return deterministicPickChannel(userKey, candidateIDs), true
	}

	assigned := a.pickLeastUsedChannelLocked(userKey, candidateIDs)
	if assigned == 0 {
		return 0, false
	}
	a.userToChannel[userKey] = userChannelAssignment{
		channelID: assigned,
		lastUsed:  now,
	}
	a.channelUseCount[assigned]++
	return assigned, true
}

func (a *userChannelAllocator) pickLeastUsedChannelLocked(userKey string, candidateIDs []int) int {
	minCount := int(^uint(0) >> 1)
	bestIDs := make([]int, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		count := a.channelUseCount[id]
		if count < minCount {
			minCount = count
			bestIDs = []int{id}
		} else if count == minCount {
			bestIDs = append(bestIDs, id)
		}
	}
	if len(bestIDs) == 0 {
		return 0
	}
	sort.Ints(bestIDs)
	return bestIDs[int(crc32.ChecksumIEEE([]byte(userKey))%uint32(len(bestIDs)))]
}

func (a *userChannelAllocator) cleanupExpiredLocked(now time.Time, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	if !a.lastCleanup.IsZero() && now.Sub(a.lastCleanup) < time.Minute {
		return
	}
	a.lastCleanup = now

	for userKey, entry := range a.userToChannel {
		if ttlExpired(entry.lastUsed, now, ttl) {
			delete(a.userToChannel, userKey)
			a.decrementChannelUseLocked(entry.channelID)
		}
	}
}

func (a *userChannelAllocator) decrementChannelUseLocked(channelID int) {
	if channelID == 0 {
		return
	}
	if count, ok := a.channelUseCount[channelID]; ok {
		count--
		if count <= 0 {
			delete(a.channelUseCount, channelID)
			return
		}
		a.channelUseCount[channelID] = count
	}
}

func deterministicPickChannel(seed string, sortedCandidateIDs []int) int {
	if seed == "" || len(sortedCandidateIDs) == 0 {
		return 0
	}
	return sortedCandidateIDs[int(crc32.ChecksumIEEE([]byte(seed))%uint32(len(sortedCandidateIDs)))]
}

func ttlExpired(lastUsed time.Time, now time.Time, ttl time.Duration) bool {
	if ttl <= 0 || lastUsed.IsZero() {
		return false
	}
	return now.Sub(lastUsed) > ttl
}

func containsInt(sortedIDs []int, target int) bool {
	idx := sort.SearchInts(sortedIDs, target)
	return idx < len(sortedIDs) && sortedIDs[idx] == target
}
