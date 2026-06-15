package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

func TestNormalizeSeedanceAssetStatus(t *testing.T) {
	cases := map[string]string{
		"created":    SeedanceProviderAssetStatusProcessing,
		"processing": SeedanceProviderAssetStatusProcessing,
		"reviewing":  SeedanceProviderAssetStatusProcessing,
		"active":     SeedanceProviderAssetStatusActive,
		"completed":  SeedanceProviderAssetStatusActive,
		"rejected":   SeedanceProviderAssetStatusFailed,
		"unexpected": SeedanceProviderAssetStatusProcessing,
	}
	for input, want := range cases {
		if got := NormalizeSeedanceAssetStatus(input); got != want {
			t.Fatalf("NormalizeSeedanceAssetStatus(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUsableSeedanceAssetURI(t *testing.T) {
	if !UsableSeedanceAssetURI("asset://provider-001") {
		t.Fatal("expected trusted asset URI to be usable")
	}
	if UsableSeedanceAssetURI("asset://mock_provider-001") {
		t.Fatal("mock asset URI should not be usable")
	}
	if UsableSeedanceAssetURI("https://example.com/image.jpg") {
		t.Fatal("public URL should not be treated as asset URI")
	}
}

func TestNewSeedanceAssetClientFromChannelSettingsDisabledWithoutCredentials(t *testing.T) {
	client := NewSeedanceAssetClientFromChannelSettings(dto.ChannelOtherSettings{})
	if _, ok := client.(DisabledSeedanceAssetClient); !ok {
		t.Fatalf("client = %T, want DisabledSeedanceAssetClient", client)
	}
}

func TestNewSeedanceAssetClientFromChannelSettingsUsesChannelCredentials(t *testing.T) {
	client := NewSeedanceAssetClientFromChannelSettings(dto.ChannelOtherSettings{
		SeedanceAssetAccessKey: "ak",
		SeedanceAssetSecretKey: "sk",
		SeedanceAssetGroupID:   "group",
	})
	if _, ok := client.(*VolcSeedanceAssetClient); !ok {
		t.Fatalf("client = %T, want *VolcSeedanceAssetClient", client)
	}
}

func TestVolcSeedanceAssetClientCreateAssetRequiresChannelGroupID(t *testing.T) {
	client := &VolcSeedanceAssetClient{}
	result, err := client.CreateAsset(t.Context(), "https://example.com/person.jpg")
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}
	if result.Status != SeedanceProviderAssetStatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.ErrorCode != "provider_asset_group_missing" {
		t.Fatalf("error code = %q, want provider_asset_group_missing", result.ErrorCode)
	}
}

func TestVolcSeedanceAssetClientDeleteAsset(t *testing.T) {
	var sawDelete bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") == "DeleteAsset" {
			sawDelete = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"Action":"DeleteAsset"},"Result":{}}`))
	}))
	defer srv.Close()

	client := NewSeedanceAssetClientFromChannelSettings(dto.ChannelOtherSettings{
		SeedanceAssetAccessKey:   "ak",
		SeedanceAssetSecretKey:   "sk",
		SeedanceAssetProjectName: "zjzx",
		SeedanceAssetEndpoint:    srv.URL,
	})
	err := client.DeleteAsset(t.Context(), "asset-001")
	if err != nil {
		t.Fatalf("DeleteAsset() error = %v", err)
	}
	if !sawDelete {
		t.Fatal("DeleteAsset action was not sent")
	}
}

func TestApplySeedanceAssetResultPreservesExistingAssetIDWhenMissing(t *testing.T) {
	ref := &model.SeedanceAssetReference{
		AssetID: "asset-001",
		URI:     "asset://asset-001",
		Status:  SeedanceProviderAssetStatusProcessing,
	}
	ApplySeedanceAssetResult(ref, &SeedanceAssetResult{
		Status:   SeedanceProviderAssetStatusActive,
		AssetID:  "",
		URI:      "",
		SyncedAt: time.Now(),
	})
	if ref.AssetID != "asset-001" {
		t.Fatalf("asset id = %q, want preserved", ref.AssetID)
	}
	if ref.URI != "asset://asset-001" {
		t.Fatalf("uri = %q, want preserved", ref.URI)
	}
	if ref.Status != SeedanceProviderAssetStatusActive {
		t.Fatalf("status = %q, want active", ref.Status)
	}
}
