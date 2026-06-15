package doubao

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestNeedsSeedanceAssetPrepareCollectsDirectImageURLs(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "make a video",
		Images: []string{"https://example.com/person.jpg", "asset://already-trusted"},
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]interface{}{
						"url": "https://example.com/scene.jpg",
					},
				},
			},
		},
	}

	prepare, ok := NeedsSeedanceAssetPrepare(req, req.Model)
	if !ok {
		t.Fatal("NeedsSeedanceAssetPrepare() = false, want true")
	}
	if len(prepare.References) != 2 {
		t.Fatalf("references len = %d, want 2", len(prepare.References))
	}
	if prepare.References[0].SourceURL != "https://example.com/person.jpg" {
		t.Fatalf("first source = %q", prepare.References[0].SourceURL)
	}
	if prepare.References[1].SourceURL != "https://example.com/scene.jpg" {
		t.Fatalf("second source = %q", prepare.References[1].SourceURL)
	}
}

func TestNeedsSeedanceAssetPrepareIgnoresNonSeedance2(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-1-5-pro-251215",
		Prompt: "make a video",
		Images: []string{"https://example.com/person.jpg"},
	}
	if _, ok := NeedsSeedanceAssetPrepare(req, req.Model); ok {
		t.Fatal("NeedsSeedanceAssetPrepare() = true, want false")
	}
}
