package model

import (
	"one-api/dto"
	"testing"
)

func TestChannelSupportsMessagesToResponsesCompatModel_AcceptsExplicitSameNameMapping(t *testing.T) {
	channel := &Channel{
		ModelMapping: strPtr(`{"claude-3-5-sonnet":"claude-3-5-sonnet"}`),
	}
	channel.SetSetting(channelCompatSetting(true))

	ok, err := channelSupportsMessagesToResponsesCompatModel(channel, "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("channelSupportsMessagesToResponsesCompatModel error: %v", err)
	}
	if !ok {
		t.Fatalf("expected explicit same-name mapping to enable compat selection")
	}
}

func TestChannelSupportsMessagesToResponsesCompatModel_AcceptsDeclaredModelWithoutMapping(t *testing.T) {
	channel := &Channel{
		Models: "claude-3-5-sonnet,gpt-5.2",
	}
	channel.SetSetting(channelCompatSetting(true))

	ok, err := channelSupportsMessagesToResponsesCompatModel(channel, "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("channelSupportsMessagesToResponsesCompatModel error: %v", err)
	}
	if !ok {
		t.Fatalf("expected declared model to enable compat selection")
	}
}

func TestChannelSupportsMessagesToResponsesCompatModel_RejectsUnsupportedModel(t *testing.T) {
	channel := &Channel{
		Models:       "gpt-5.2",
		ModelMapping: strPtr(`{"claude-3-7-sonnet":"gpt-5.2"}`),
	}
	channel.SetSetting(channelCompatSetting(true))

	ok, err := channelSupportsMessagesToResponsesCompatModel(channel, "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("channelSupportsMessagesToResponsesCompatModel error: %v", err)
	}
	if ok {
		t.Fatalf("expected unsupported model to be rejected")
	}
}

func channelCompatSetting(enabled bool) dto.ChannelSettings {
	return dto.ChannelSettings{MessagesToResponsesCompat: enabled}
}

func strPtr(v string) *string {
	return &v
}
