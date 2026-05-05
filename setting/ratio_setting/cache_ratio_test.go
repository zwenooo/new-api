package ratio_setting

import "testing"

func TestGPT5CreateCacheRatioFallbacks(t *testing.T) {
	ratio, ok := GetCreateCacheRatio("gpt-5.4")
	if !ok || ratio != 1 {
		t.Fatalf("GetCreateCacheRatio(gpt-5.4) = %v,%v want 1,true", ratio, ok)
	}
}

func TestCacheRatioMatchesReasoningSuffixModel(t *testing.T) {
	InitRatioSettings()

	cacheRatio, ok := GetCacheRatio("gpt-5.4-high")
	if !ok || cacheRatio != 0.1 {
		t.Fatalf("GetCacheRatio(gpt-5.4-high) = %v,%v want 0.1,true", cacheRatio, ok)
	}

	createRatio, ok := GetCreateCacheRatio("gpt-5.4-high")
	if !ok || createRatio != 1 {
		t.Fatalf("GetCreateCacheRatio(gpt-5.4-high) = %v,%v want 1,true", createRatio, ok)
	}
}

func TestUpdateCreateCacheRatioByJSONString(t *testing.T) {
	InitRatioSettings()

	if err := UpdateCreateCacheRatioByJSONString(`{"gemini-2.5-pro":0.3}`); err != nil {
		t.Fatalf("UpdateCreateCacheRatioByJSONString() error = %v", err)
	}

	ratio, ok := GetCreateCacheRatio("gemini-2.5-pro")
	if !ok || ratio != 0.3 {
		t.Fatalf("GetCreateCacheRatio(gemini-2.5-pro) = %v,%v want 0.3,true", ratio, ok)
	}
}
