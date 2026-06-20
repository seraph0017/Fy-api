package deepseek

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestApplyDeepSeekV4OpenAIThinkingSuffix(t *testing.T) {
	t.Parallel()

	t.Run("nothink alias maps to disabled thinking", func(t *testing.T) {
		t.Parallel()

		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "deepseek-v4-pro-nothink",
			},
		}
		request := &dto.GeneralOpenAIRequest{Model: "deepseek-v4-pro-nothink"}

		err := applyDeepSeekV4OpenAIThinkingSuffix(info, request)
		require.NoError(t, err)
		require.Equal(t, "deepseek-v4-pro", request.Model)
		require.Equal(t, "deepseek-v4-pro", info.UpstreamModelName)
		require.Empty(t, request.ReasoningEffort)
		require.Empty(t, info.ReasoningEffort)

		var thinking map[string]string
		err = common.Unmarshal(request.THINKING, &thinking)
		require.NoError(t, err)
		require.Equal(t, "disabled", thinking["type"])
	})

	t.Run("max suffix keeps max effort", func(t *testing.T) {
		t.Parallel()

		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "deepseek-v4-flash-max",
			},
		}
		request := &dto.GeneralOpenAIRequest{Model: "deepseek-v4-flash-max"}

		err := applyDeepSeekV4OpenAIThinkingSuffix(info, request)
		require.NoError(t, err)
		require.Equal(t, "deepseek-v4-flash", request.Model)
		require.Equal(t, "deepseek-v4-flash", info.UpstreamModelName)
		require.Equal(t, "max", request.ReasoningEffort)
		require.Equal(t, "max", info.ReasoningEffort)

		var thinking map[string]string
		err = common.Unmarshal(request.THINKING, &thinking)
		require.NoError(t, err)
		require.Equal(t, "enabled", thinking["type"])
	})
}

func TestApplyDeepSeekV4ClaudeThinkingSuffix(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-v4-pro-nothinking",
		},
	}
	request := &dto.ClaudeRequest{Model: "deepseek-v4-pro-nothinking"}

	err := applyDeepSeekV4ClaudeThinkingSuffix(info, request)
	require.NoError(t, err)
	require.Equal(t, "deepseek-v4-pro", request.Model)
	require.Equal(t, "deepseek-v4-pro", info.UpstreamModelName)
	require.NotNil(t, request.Thinking)
	require.Equal(t, "disabled", request.Thinking.Type)
	require.Nil(t, request.OutputConfig)
}
