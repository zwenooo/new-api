package relay

import (
	"net/http/httptest"
	"testing"

	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/setting/model_setting"

	"github.com/gin-gonic/gin"
)

func TestShouldPassThroughClaudeRequest(t *testing.T) {
	backup := *model_setting.GetGlobalSettings()
	t.Cleanup(func() {
		*model_setting.GetGlobalSettings() = backup
	})

	tests := []struct {
		name               string
		globalPassThrough  bool
		channelPassThrough bool
		channelCompat      bool
		wantPassThrough    bool
	}{
		{
			name:            "disabled by default",
			wantPassThrough: false,
		},
		{
			name:              "global passthrough for normal channel",
			globalPassThrough: true,
			wantPassThrough:   true,
		},
		{
			name:               "channel passthrough for normal channel",
			channelPassThrough: true,
			wantPassThrough:    true,
		},
		{
			name:              "global passthrough must not bypass channel messages responses compat",
			globalPassThrough: true,
			channelCompat:     true,
			wantPassThrough:   false,
		},
		{
			name:               "channel passthrough must not bypass channel messages responses compat",
			channelPassThrough: true,
			channelCompat:      true,
			wantPassThrough:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*model_setting.GetGlobalSettings() = backup
			model_setting.GetGlobalSettings().PassThroughRequestEnabled = tt.globalPassThrough

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(c, constant.ContextKeyChannelMessagesToResponsesCompat, tt.channelCompat)

			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelSetting: dto.ChannelSettings{
						PassThroughBodyEnabled: tt.channelPassThrough,
					},
				},
			}

			if got := shouldPassThroughClaudeRequest(c, info); got != tt.wantPassThrough {
				t.Fatalf("shouldPassThroughClaudeRequest() = %v, want %v", got, tt.wantPassThrough)
			}
		})
	}
}
