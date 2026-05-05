package model

func normalizeOrderedUniquePositiveIDs(rawIDs []int) []int {
	if len(rawIDs) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(rawIDs))
	ordered := make([]int, 0, len(rawIDs))
	for _, id := range rawIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}
	return ordered
}

func normalizeOrderedUniqueNonZeroIDs(rawIDs []int) []int {
	if len(rawIDs) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(rawIDs))
	ordered := make([]int, 0, len(rawIDs))
	for _, id := range rawIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}
	return ordered
}
