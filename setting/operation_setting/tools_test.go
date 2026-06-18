package operation_setting

import "testing"

func TestResolveImageGenerationPrice(t *testing.T) {
	got := ResolveImageGenerationPrice("low", "1024x1536")
	if got.Price != GPTImage1Low1024x1536 {
		t.Fatalf("expected low 1024x1536 price %v, got %v", GPTImage1Low1024x1536, got.Price)
	}
	if got.Quality != "low" || got.Size != "1024x1536" || got.UsedDefaultPrice {
		t.Fatalf("expected exact match metadata, got %#v", got)
	}

	fallback := ResolveImageGenerationPrice("auto", "2048x2048")
	if fallback.Price != GPTImage1High1024x1024 {
		t.Fatalf("expected fallback price %v, got %v", GPTImage1High1024x1024, fallback.Price)
	}
	if fallback.Quality != "high" || fallback.Size != "1024x1024" || !fallback.UsedDefaultPrice {
		t.Fatalf("expected default fallback metadata, got %#v", fallback)
	}
}
