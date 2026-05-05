package middleware

import "one-api/model"

type groupChannelSelectionError struct {
	GroupID     int
	Step        string
	LookupModel string
	Err         error
}

type groupChannelSelectionStep struct {
	kind   string
	lookup func() (*model.Channel, string, error)
}

func selectChannelByCandidateGroupOrder(
	candidateGroupIDs []int,
	allowUserAgent func(int) bool,
	stepsForGroup func(int) []groupChannelSelectionStep,
) (*model.Channel, int, bool, *groupChannelSelectionError) {
	uaAcceptedAny := false
	var deferredErr *groupChannelSelectionError
	for _, gid := range candidateGroupIDs {
		if gid <= 0 {
			continue
		}
		if !allowUserAgent(gid) {
			continue
		}
		uaAcceptedAny = true
		steps := stepsForGroup(gid)
		for _, step := range steps {
			if step.lookup == nil {
				continue
			}
			channel, lookupModel, err := step.lookup()
			if err != nil {
				if model.IsChannelConcurrencyLimitReachedErr(err) {
					if deferredErr == nil {
						deferredErr = &groupChannelSelectionError{
							GroupID:     gid,
							Step:        step.kind,
							LookupModel: lookupModel,
							Err:         err,
						}
					}
					continue
				}
				return nil, 0, uaAcceptedAny, &groupChannelSelectionError{
					GroupID:     gid,
					Step:        step.kind,
					LookupModel: lookupModel,
					Err:         err,
				}
			}
			if channel != nil {
				return channel, gid, uaAcceptedAny, nil
			}
		}
	}
	return nil, 0, uaAcceptedAny, deferredErr
}
