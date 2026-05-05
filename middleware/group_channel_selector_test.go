package middleware

import (
	"fmt"
	"testing"

	"one-api/model"
)

func TestSelectChannelByCandidateGroupOrder_PrefersMappedChannelInEarlierGroup(t *testing.T) {
	steps := func(groupID int) []groupChannelSelectionStep {
		switch groupID {
		case 1:
			return []groupChannelSelectionStep{
				{
					kind: "direct",
					lookup: func() (*model.Channel, string, error) {
						return nil, "claude-sonnet", nil
					},
				},
				{
					kind: "messages_to_responses_compat",
					lookup: func() (*model.Channel, string, error) {
						return nil, "claude-sonnet", nil
					},
				},
				{
					kind: "messages_to_responses_compat",
					lookup: func() (*model.Channel, string, error) {
						return &model.Channel{Id: 101}, "gpt-4.1", nil
					},
				},
			}
		case 3:
			return []groupChannelSelectionStep{
				{
					kind: "direct",
					lookup: func() (*model.Channel, string, error) {
						t.Fatal("later group's direct channel should not run before earlier group's mapped fallback")
						return nil, "", nil
					},
				},
			}
		default:
			return nil
		}
	}

	channel, groupID, uaAcceptedAny, err := selectChannelByCandidateGroupOrder(
		[]int{1, 3},
		func(int) bool { return true },
		steps,
	)
	if err != nil {
		t.Fatalf("selectChannelByCandidateGroupOrder() unexpected error: %+v", err)
	}
	if !uaAcceptedAny {
		t.Fatal("selectChannelByCandidateGroupOrder() should mark ua as accepted")
	}
	if groupID != 1 {
		t.Fatalf("selectChannelByCandidateGroupOrder() groupID = %d, want 1", groupID)
	}
	if channel == nil || channel.Id != 101 {
		t.Fatalf("selectChannelByCandidateGroupOrder() channel = %+v, want channel id 101", channel)
	}
}

func TestSelectChannelByCandidateGroupOrder_FallsBackToLaterGroupAfterEarlierGroupExhausted(t *testing.T) {
	callOrder := make([]string, 0, 4)
	steps := func(groupID int) []groupChannelSelectionStep {
		switch groupID {
		case 1:
			return []groupChannelSelectionStep{
				{
					kind: "direct",
					lookup: func() (*model.Channel, string, error) {
						callOrder = append(callOrder, "1:direct")
						return nil, "claude-sonnet", nil
					},
				},
				{
					kind: "messages_to_responses_compat",
					lookup: func() (*model.Channel, string, error) {
						callOrder = append(callOrder, "1:mapped")
						return nil, "gpt-4.1", nil
					},
				},
			}
		case 3:
			return []groupChannelSelectionStep{
				{
					kind: "direct",
					lookup: func() (*model.Channel, string, error) {
						callOrder = append(callOrder, "3:direct")
						return &model.Channel{Id: 303}, "claude-sonnet", nil
					},
				},
			}
		default:
			return nil
		}
	}

	channel, groupID, uaAcceptedAny, err := selectChannelByCandidateGroupOrder(
		[]int{1, 3},
		func(int) bool { return true },
		steps,
	)
	if err != nil {
		t.Fatalf("selectChannelByCandidateGroupOrder() unexpected error: %+v", err)
	}
	if !uaAcceptedAny {
		t.Fatal("selectChannelByCandidateGroupOrder() should mark ua as accepted")
	}
	if groupID != 3 {
		t.Fatalf("selectChannelByCandidateGroupOrder() groupID = %d, want 3", groupID)
	}
	if channel == nil || channel.Id != 303 {
		t.Fatalf("selectChannelByCandidateGroupOrder() channel = %+v, want channel id 303", channel)
	}

	gotOrder := fmt.Sprintf("%v", callOrder)
	wantOrder := "[1:direct 1:mapped 3:direct]"
	if gotOrder != wantOrder {
		t.Fatalf("selectChannelByCandidateGroupOrder() call order = %s, want %s", gotOrder, wantOrder)
	}
}
