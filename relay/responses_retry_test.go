package relay

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
)

func TestShouldRetryWithoutEncryptedContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    *types.NewAPIError
		expect bool
	}{
		{
			name: "matches encrypted content verification failure",
			err: types.NewOpenAIError(
				errors.New("The encrypted content gAAA... could not be verified. Reason: Encrypted content could not be decrypted or parsed."),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusBadRequest,
			),
			expect: true,
		},
		{
			name: "ignores other 400 errors",
			err: types.NewOpenAIError(
				errors.New("Unknown parameter: 'input[9].metadata'."),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusBadRequest,
			),
			expect: false,
		},
		{
			name: "ignores non-400 errors",
			err: types.NewOpenAIError(
				errors.New("The encrypted content gAAA... could not be verified. Reason: Encrypted content could not be decrypted or parsed."),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusInternalServerError,
			),
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expect, shouldRetryWithoutEncryptedContent(tt.err))
		})
	}
}
