package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestAppendNewAPIErrorMetadataToAdminInfoAddsUpstreamDebug(t *testing.T) {
	metadata, err := common.Marshal(map[string]interface{}{
		"upstream_debug": map[string]interface{}{
			"host":        "api.apipro.ai",
			"status_code": 504,
		},
	})
	require.NoError(t, err)
	apiErr := &types.NewAPIError{Metadata: metadata}
	adminInfo := map[string]interface{}{}

	appendNewAPIErrorMetadataToAdminInfo(apiErr, adminInfo)

	debug, ok := adminInfo["upstream_debug"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "api.apipro.ai", debug["host"])
	require.Equal(t, float64(504), debug["status_code"])
}

func TestAppendNewAPIErrorMetadataToAdminInfoIgnoresUnknownMetadata(t *testing.T) {
	metadata, err := common.Marshal(map[string]interface{}{
		"unexpected": "value",
	})
	require.NoError(t, err)
	apiErr := &types.NewAPIError{Metadata: metadata}
	adminInfo := map[string]interface{}{}

	appendNewAPIErrorMetadataToAdminInfo(apiErr, adminInfo)

	require.NotContains(t, adminInfo, "unexpected")
	require.NotContains(t, adminInfo, "upstream_debug")
}
