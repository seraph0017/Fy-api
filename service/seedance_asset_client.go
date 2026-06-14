package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/volcengine/volcengine-go-sdk/volcengine"
	"github.com/volcengine/volcengine-go-sdk/volcengine/credentials"
	"github.com/volcengine/volcengine-go-sdk/volcengine/session"
	"github.com/volcengine/volcengine-go-sdk/volcengine/universal"
)

const (
	SeedanceProviderAssetStatusCreating   = "creating"
	SeedanceProviderAssetStatusProcessing = "processing"
	SeedanceProviderAssetStatusActive     = "active"
	SeedanceProviderAssetStatusFailed     = "failed"

	seedanceProviderAssetScheme     = "asset://"
	seedanceMockProviderAssetPrefix = "mock_"
)

type SeedanceAssetClient interface {
	CreateAsset(ctx context.Context, sourceURL string) (*SeedanceAssetResult, error)
	GetAsset(ctx context.Context, assetID string) (*SeedanceAssetResult, error)
}

type SeedanceAssetResult struct {
	Status       string
	AssetID      string
	URI          string
	ErrorCode    string
	ErrorMessage string
	SyncedAt     time.Time
}

type VolcSeedanceAssetClient struct {
	groupID     string
	projectName string
	universal   *universal.Universal
	initErr     error
}

func NewSeedanceAssetClientFromChannelSettings(settings dto.ChannelOtherSettings) SeedanceAssetClient {
	accessKey := strings.TrimSpace(settings.SeedanceAssetAccessKey)
	secretKey := strings.TrimSpace(settings.SeedanceAssetSecretKey)
	if accessKey == "" || secretKey == "" {
		return DisabledSeedanceAssetClient{}
	}
	timeoutSeconds := settings.SeedanceAssetTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 20
	}
	timeout := time.Duration(timeoutSeconds) * time.Second
	region := strings.TrimSpace(settings.SeedanceAssetRegion)
	if region == "" {
		region = "cn-beijing"
	}
	projectName := strings.TrimSpace(settings.SeedanceAssetProjectName)
	if projectName == "" {
		projectName = "default"
	}
	groupID := strings.TrimSpace(settings.SeedanceAssetGroupID)

	cfg := volcengine.NewConfig().
		WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, "")).
		WithRegion(region).
		WithHTTPClient(&http.Client{Timeout: timeout})
	if endpoint := strings.TrimSpace(settings.SeedanceAssetEndpoint); endpoint != "" {
		cfg = cfg.WithEndpoint(endpoint)
	}
	sess, err := session.NewSession(cfg)
	if err != nil {
		return &VolcSeedanceAssetClient{groupID: groupID, projectName: projectName, initErr: err}
	}
	return &VolcSeedanceAssetClient{
		groupID:     groupID,
		projectName: projectName,
		universal:   universal.New(sess),
	}
}

type DisabledSeedanceAssetClient struct{}

func (DisabledSeedanceAssetClient) CreateAsset(ctx context.Context, sourceURL string) (*SeedanceAssetResult, error) {
	return seedanceAssetFailed("provider_asset_not_configured", "当前渠道未配置 Seedance Ark Asset Service 凭证，无法创建可信素材"), nil
}

func (DisabledSeedanceAssetClient) GetAsset(ctx context.Context, assetID string) (*SeedanceAssetResult, error) {
	return seedanceAssetFailed("provider_asset_not_configured", "当前渠道未配置 Seedance Ark Asset Service 凭证，无法查询可信素材"), nil
}

func (c *VolcSeedanceAssetClient) CreateAsset(ctx context.Context, sourceURL string) (*SeedanceAssetResult, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}
	if strings.TrimSpace(sourceURL) == "" {
		return seedanceAssetFailed("missing_image_url", "参考图 URL 为空，无法创建可信素材"), nil
	}
	if c.groupID == "" {
		return seedanceAssetFailed("provider_asset_group_missing", "当前渠道未配置 Seedance Ark Asset Service Group ID，无法创建可信素材"), nil
	}
	input := map[string]interface{}{
		"GroupId":     c.groupID,
		"URL":         strings.TrimSpace(sourceURL),
		"AssetType":   "Image",
		"ProjectName": c.projectName,
	}
	output, err := c.universal.DoCall(seedanceAssetUniversalRequest("CreateAsset"), &input)
	if err != nil {
		return nil, err
	}
	return seedanceAssetPayloadFromMap(output).result(), nil
}

func (c *VolcSeedanceAssetClient) GetAsset(ctx context.Context, assetID string) (*SeedanceAssetResult, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}
	if strings.TrimSpace(assetID) == "" {
		return seedanceAssetFailed("missing_provider_asset_id", "可信素材 ID 为空，无法查询审核状态"), nil
	}
	input := map[string]interface{}{
		"Id":          strings.TrimSpace(assetID),
		"ProjectName": c.projectName,
	}
	output, err := c.universal.DoCall(seedanceAssetUniversalRequest("GetAsset"), &input)
	if err != nil {
		return nil, err
	}
	return seedanceAssetPayloadFromMap(output).result(), nil
}

func seedanceAssetUniversalRequest(action string) universal.RequestUniversal {
	return universal.RequestUniversal{
		ServiceName: "ark",
		Action:      action,
		Version:     "2024-01-01",
		HttpMethod:  universal.POST,
		ContentType: universal.ApplicationJSON,
	}
}

func seedanceAssetFailed(code, message string) *SeedanceAssetResult {
	return &SeedanceAssetResult{
		Status:       SeedanceProviderAssetStatusFailed,
		ErrorCode:    code,
		ErrorMessage: message,
		SyncedAt:     time.Now(),
	}
}

type seedanceAssetPayloadFields struct {
	Status       string `json:"status"`
	VolcStatus   string `json:"Status"`
	AssetID      string `json:"asset_id"`
	ID           string `json:"id"`
	VolcID       string `json:"Id"`
	URI          string `json:"uri"`
	ProviderURI  string `json:"provider_asset_uri"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Error        *struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	} `json:"Error"`
}

type seedanceAssetPayload struct {
	seedanceAssetPayloadFields
	Result *seedanceAssetPayloadFields `json:"Result"`
	Data   *seedanceAssetPayloadFields `json:"data"`
}

func seedanceAssetPayloadFromMap(output *map[string]interface{}) seedanceAssetPayload {
	if output == nil {
		return seedanceAssetPayload{}
	}
	b, err := common.Marshal(output)
	if err != nil {
		return seedanceAssetPayload{}
	}
	var payload seedanceAssetPayload
	_ = common.Unmarshal(b, &payload)
	return payload
}

func (f seedanceAssetPayloadFields) mergeInto(status, assetID, uri, errorCode, errorMessage string) (string, string, string, string, string) {
	if status == "" {
		status = f.Status
	}
	if status == "" {
		status = f.VolcStatus
	}
	if assetID == "" {
		assetID = f.AssetID
	}
	if assetID == "" {
		assetID = f.ID
	}
	if assetID == "" {
		assetID = f.VolcID
	}
	if uri == "" {
		uri = f.URI
	}
	if uri == "" {
		uri = f.ProviderURI
	}
	if errorCode == "" {
		errorCode = f.ErrorCode
	}
	if errorMessage == "" {
		errorMessage = f.ErrorMessage
	}
	if f.Error != nil {
		if f.Error.Code != "" {
			errorCode = f.Error.Code
		}
		if f.Error.Message != "" {
			errorMessage = f.Error.Message
		}
	}
	return status, assetID, uri, errorCode, errorMessage
}

func (p seedanceAssetPayload) result() *SeedanceAssetResult {
	status, assetID, uri, errorCode, errorMessage := p.seedanceAssetPayloadFields.mergeInto("", "", "", "", "")
	if p.Result != nil {
		status, assetID, uri, errorCode, errorMessage = p.Result.mergeInto(status, assetID, uri, errorCode, errorMessage)
	}
	if p.Data != nil {
		status, assetID, uri, errorCode, errorMessage = p.Data.mergeInto(status, assetID, uri, errorCode, errorMessage)
	}
	normalizedStatus := NormalizeSeedanceAssetStatus(status)
	if uri == "" && assetID != "" && normalizedStatus == SeedanceProviderAssetStatusActive {
		uri = fmt.Sprintf("%s%s", seedanceProviderAssetScheme, assetID)
	}
	return &SeedanceAssetResult{
		Status:       normalizedStatus,
		AssetID:      assetID,
		URI:          uri,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		SyncedAt:     time.Now(),
	}
}

func NormalizeSeedanceAssetStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case SeedanceProviderAssetStatusCreating:
		return SeedanceProviderAssetStatusCreating
	case SeedanceProviderAssetStatusProcessing, "reviewing", "pending", "":
		return SeedanceProviderAssetStatusProcessing
	case SeedanceProviderAssetStatusActive, "ready", "success", "completed":
		return SeedanceProviderAssetStatusActive
	case SeedanceProviderAssetStatusFailed, "rejected", "expired", "error":
		return SeedanceProviderAssetStatusFailed
	default:
		return SeedanceProviderAssetStatusProcessing
	}
}

func UsableSeedanceAssetURI(uri string) bool {
	trimmed := strings.TrimSpace(uri)
	return strings.HasPrefix(trimmed, seedanceProviderAssetScheme) &&
		!strings.HasPrefix(strings.TrimPrefix(trimmed, seedanceProviderAssetScheme), seedanceMockProviderAssetPrefix)
}

func ApplySeedanceAssetResult(ref *model.SeedanceAssetReference, result *SeedanceAssetResult) {
	if ref == nil || result == nil {
		return
	}
	if result.Status != "" {
		ref.Status = result.Status
	}
	if result.AssetID != "" {
		ref.AssetID = result.AssetID
	}
	if result.URI != "" {
		ref.URI = result.URI
	}
	ref.ErrorCode = result.ErrorCode
	ref.ErrorMessage = result.ErrorMessage
	ref.SyncedAt = result.SyncedAt.Unix()
}
