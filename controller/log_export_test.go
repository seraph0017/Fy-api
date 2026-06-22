package controller

import (
	"encoding/csv"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteLogsCSVIncludesCacheTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeLogsCSV(ctx, []*model.Log{
		{
			CreatedAt:        1710000000,
			ChannelId:        3,
			ChannelName:      "anthropic",
			Username:         "alice",
			TokenName:        "prod",
			Group:            "default",
			Type:             model.LogTypeConsume,
			ModelName:        "claude-sonnet",
			PromptTokens:     100,
			CompletionTokens: 20,
			Quota:            500,
			RequestId:        "req_123",
			Content:          "ok",
			Other:            common.MapToJsonStr(map[string]interface{}{"cache_tokens": 30, "cache_creation_tokens_5m": 7, "cache_creation_tokens_1h": 11}),
		},
	})

	body := strings.TrimPrefix(recorder.Body.String(), "\ufeff")
	rows, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)

	assert.Equal(t, []string{
		"时间", "渠道", "用户", "令牌", "分组", "类型", "模型",
		"用时/首字", "输入", "缓存读", "缓存写", "输出", "Quota", "人民币", "美金", "IP", "重试", "request_id", "详情",
	}, rows[0])
	assert.Equal(t, "100", rows[1][8])
	assert.Equal(t, "30", rows[1][9])
	assert.Equal(t, "18", rows[1][10])
	assert.Equal(t, "20", rows[1][11])
}

func TestGetLogCacheTokenSummaryUsesCacheWriteTotal(t *testing.T) {
	summary := getLogCacheTokenSummary(common.MapToJsonStr(map[string]interface{}{
		"cache_tokens":             12,
		"cache_write_tokens":       34,
		"cache_creation_tokens":    56,
		"cache_creation_tokens_5m": 78,
		"cache_creation_tokens_1h": 90,
	}))

	assert.Equal(t, 12, summary.cacheReadTokens)
	assert.Equal(t, 34, summary.cacheWriteTokens)
}
