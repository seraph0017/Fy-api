package doubao

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

const SeedanceAssetPrepareIDPrefix = "asset_prepare:"

func NeedsSeedanceAssetPrepare(req relaycommon.TaskSubmitReq, modelName string) (*model.SeedanceAssetPrepareData, bool) {
	if !IsSeedance2Model(modelName) {
		return nil, false
	}
	refs := collectSeedanceImageReferences(req)
	if len(refs) == 0 {
		return nil, false
	}
	reqCopy := req
	return &model.SeedanceAssetPrepareData{
		Stage:      model.SeedanceAssetPrepareStageCreating,
		Request:    &reqCopy,
		References: refs,
	}, true
}

func IsSeedance2Model(modelName string) bool {
	return strings.HasPrefix(modelName, "doubao-seedance-2-0-") || strings.HasPrefix(modelName, "seedance-2-0-")
}

func collectSeedanceImageReferences(req relaycommon.TaskSubmitReq) []model.SeedanceAssetReference {
	seen := map[string]bool{}
	refs := make([]model.SeedanceAssetReference, 0)
	add := func(url string) {
		url = strings.TrimSpace(url)
		if !needsSeedanceProviderAsset(url) || seen[url] {
			return
		}
		seen[url] = true
		refs = append(refs, model.SeedanceAssetReference{
			Ordinal:   len(refs),
			SourceURL: url,
			Status:    service.SeedanceProviderAssetStatusCreating,
		})
	}
	for _, img := range req.Images {
		add(img)
	}
	if req.Image != "" {
		add(req.Image)
	}
	if req.InputReference != "" {
		add(req.InputReference)
	}
	collectSeedanceMetadataImageReferences(req.Metadata, add)
	return refs
}

func collectSeedanceMetadataImageReferences(v any, add func(string)) {
	switch x := v.(type) {
	case map[string]interface{}:
		if imageURL, ok := x["image_url"]; ok {
			switch iv := imageURL.(type) {
			case string:
				add(iv)
			case map[string]interface{}:
				if url, _ := iv["url"].(string); url != "" {
					add(url)
				}
			}
		}
		for _, child := range x {
			collectSeedanceMetadataImageReferences(child, add)
		}
	case []interface{}:
		for _, child := range x {
			collectSeedanceMetadataImageReferences(child, add)
		}
	}
}

func needsSeedanceProviderAsset(url string) bool {
	url = strings.TrimSpace(url)
	return (strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) &&
		!strings.HasPrefix(url, "asset://")
}

func RewriteSeedanceRequestWithAssets(req relaycommon.TaskSubmitReq, refs []model.SeedanceAssetReference) relaycommon.TaskSubmitReq {
	replacements := map[string]string{}
	for _, ref := range refs {
		if ref.SourceURL != "" && service.UsableSeedanceAssetURI(ref.URI) {
			replacements[strings.TrimSpace(ref.SourceURL)] = ref.URI
		}
	}
	if len(replacements) == 0 {
		return req
	}
	replace := func(url string) string {
		if uri, ok := replacements[strings.TrimSpace(url)]; ok {
			return uri
		}
		return url
	}
	for i := range req.Images {
		req.Images[i] = replace(req.Images[i])
	}
	req.Image = replace(req.Image)
	req.InputReference = replace(req.InputReference)
	if req.Metadata != nil {
		if rewritten, ok := rewriteSeedanceMetadataAssets(req.Metadata, replacements).(map[string]interface{}); ok {
			req.Metadata = rewritten
		}
	}
	return req
}

func rewriteSeedanceMetadataAssets(v any, replacements map[string]string) any {
	switch x := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, child := range x {
			if k == "url" {
				if s, ok := child.(string); ok {
					if uri, found := replacements[strings.TrimSpace(s)]; found {
						out[k] = uri
						continue
					}
				}
			}
			out[k] = rewriteSeedanceMetadataAssets(child, replacements)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, child := range x {
			out[i] = rewriteSeedanceMetadataAssets(child, replacements)
		}
		return out
	default:
		return v
	}
}

func (a *TaskAdaptor) SubmitPreparedTask(baseURL string, key string, proxy string, req relaycommon.TaskSubmitReq, upstreamModelName string) (string, []byte, error) {
	payload, err := a.convertToRequestPayload(&req)
	if err != nil {
		return "", nil, fmt.Errorf("convert request payload failed: %w", err)
	}
	if upstreamModelName != "" {
		payload.Model = upstreamModelName
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	uri := fmt.Sprintf("%s/api/v3/contents/generations/tasks", baseURL)
	httpReq, err := http.NewRequest(http.MethodPost, uri, bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return "", nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", respBody, fmt.Errorf("seedance submit failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	var dResp responsePayload
	if err := common.Unmarshal(respBody, &dResp); err != nil {
		return "", respBody, err
	}
	if dResp.ID == "" {
		return "", respBody, fmt.Errorf("task_id is empty")
	}
	return dResp.ID, respBody, nil
}

func SeedanceAssetPrepareProgress(stage string) string {
	switch stage {
	case model.SeedanceAssetPrepareStageCreating:
		return "5%"
	case model.SeedanceAssetPrepareStageProcessing, model.SeedanceAssetPrepareStageActive:
		return "15%"
	case model.SeedanceAssetPrepareStageSubmitted:
		return "20%"
	default:
		return "10%"
	}
}

func SeedanceAssetPrepareTaskData(publicTaskID, modelName string) []byte {
	ov := map[string]any{
		"id":         publicTaskID,
		"task_id":    publicTaskID,
		"object":     "video",
		"created_at": time.Now().Unix(),
		"model":      modelName,
		"status":     "queued",
		"progress":   0,
	}
	data, _ := common.Marshal(ov)
	return data
}
