package relay

import (
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func shouldRetryWithoutEncryptedContent(err *types.NewAPIError) bool {
	if err == nil || err.StatusCode != http.StatusBadRequest {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "encrypted content") &&
		strings.Contains(msg, "could not be verified") &&
		strings.Contains(msg, "could not be decrypted or parsed")
}

func retryResponsesRequestWithoutEncryptedContent(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	adaptor channel.Adaptor,
	requestJSON []byte,
	upstreamErr *types.NewAPIError,
) (*http.Response, *types.NewAPIError, bool) {
	if !shouldRetryWithoutEncryptedContent(upstreamErr) {
		return nil, nil, false
	}

	// Fy-api overlay: do not try to interpret or repair the encrypted payload.
	// Only perform a single degraded retry with encrypted_content removed.
	retryJSON, changed, err := relaycommon.StripEncryptedContentFromOpenAIResponsesRequest(requestJSON)
	if err != nil || !changed {
		return nil, nil, false
	}

	logger.LogInfo(c, "retrying responses request without encrypted_content after upstream verification failure")

	body, size, closer, err := relaycommon.NewOutboundJSONBody(retryJSON)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry()), true
	}
	defer closer.Close()
	info.UpstreamRequestBodySize = size

	resp, err := adaptor.DoRequest(c, info, body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError), true
	}
	if resp == nil {
		return nil, types.NewOpenAIError(nil, types.ErrorCodeBadResponse, http.StatusInternalServerError), true
	}

	httpResp := resp.(*http.Response)
	if httpResp.StatusCode != http.StatusOK {
		return nil, service.RelayErrorHandler(c.Request.Context(), httpResp, false), true
	}
	return httpResp, nil, true
}

func outboundRequestJSONFromBodyStorage(storage io.Reader) []byte {
	if bodyStorage, ok := storage.(interface{ Bytes() ([]byte, error) }); ok {
		if b, err := bodyStorage.Bytes(); err == nil {
			return append([]byte(nil), b...)
		}
	}
	return nil
}
