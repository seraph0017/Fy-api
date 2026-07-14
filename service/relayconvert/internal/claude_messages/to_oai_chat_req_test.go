package claudemessages

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/require"
)

func TestClaudeMessagesRequestToOpenAIChatRejectsImageWithoutSource(t *testing.T) {
	request := dto.ClaudeRequest{
		Model: "claude-3-5-sonnet-20240620",
		Messages: []dto.ClaudeMessage{
			{
				Role: "user",
			},
		},
	}
	request.Messages[0].SetContent([]dto.ClaudeMediaMessage{
		{Type: "image"},
	})

	converted, err := ClaudeMessagesRequestToOpenAIChat(request, nil)

	require.Nil(t, converted)
	require.ErrorContains(t, err, "image type requires a 'source' object")
}
