package controller

import (
	"one-api/common"
	"one-api/dto"
	"testing"
)

func TestConvertOpenRouterModelsResponse(t *testing.T) {
	body := []byte(`{
  "data": [
	    {
	      "id": "openai/gpt-5.2",
	      "architecture": {
	        "output_modalities": ["text"]
	      },
	      "pricing": {
	        "prompt": "0.00000175",
	        "completion": "0.000014",
	        "input_cache_read": "0.000000175",
	        "input_cache_write": "0.00000175"
	      }
	    },
	    {
	      "id": "anthropic/claude-opus-4.5",
	      "architecture": {
	        "output_modalities": ["text"]
	      },
	      "pricing": {
	        "prompt": "0.000005",
	        "completion": "0.000025",
	        "input_cache_read": "0.0000005",
	        "input_cache_write": "0.00000625"
	      }
	    },
	    {
	      "id": "vendor/per-call-model",
	      "architecture": {
	        "output_modalities": ["text"]
	      },
      "pricing": {
        "prompt": "0",
        "completion": "0",
	        "request": "0.04"
	      }
	    },
	    {
	      "id": "openai/gpt-4o",
	      "architecture": {
	        "output_modalities": ["text"]
	      },
	      "pricing": {
	        "prompt": "0.0000025",
	        "completion": "0.00001"
	      }
	    },
	    {
	      "id": "openai/gpt-4o:extended",
	      "architecture": {
	        "output_modalities": ["text"]
	      },
	      "pricing": {
	        "prompt": "0.000006",
	        "completion": "0.000018"
	      }
	    },
	    {
	      "id": "google/gemma-4-26b-a4b-it:free",
	      "architecture": {
	        "output_modalities": ["text"]
	      },
	      "pricing": {
	        "prompt": "0",
	        "completion": "0",
	        "input_cache_write": "0"
	      }
	    },
	    {
	      "id": "vendor/default-cache-model",
	      "architecture": {
	        "output_modalities": ["text"]
	      },
	      "pricing": {
	        "prompt": "0.000002",
	        "completion": "0.000004"
	      }
	    },
	    {
	      "id": "vendor/default-completion-and-cache-model",
	      "architecture": {
	        "output_modalities": ["text"]
	      },
	      "pricing": {
	        "prompt": "0.000002"
	      }
	    },
	    {
	      "id": "vendor/free-missing-fields",
	      "architecture": {
	        "output_modalities": ["text"]
	      },
	      "pricing": {
	        "prompt": "0"
	      }
	    },
	    {
	      "id": "vendor/audio-only-model",
	      "architecture": {
	        "output_modalities": ["audio"]
      },
      "pricing": {
        "request": "0.08"
      }
    }
  ]
}`)

	converted, err := convertOpenRouterModelsResponse(body)
	if err != nil {
		t.Fatalf("convertOpenRouterModelsResponse() error = %v", err)
	}

	modelRatioMap, ok := converted["model_ratio"].(map[string]any)
	if !ok {
		t.Fatalf("model_ratio missing or invalid: %#v", converted["model_ratio"])
	}
	if got := modelRatioMap["gpt-5.2"]; got != 0.875 {
		t.Fatalf("gpt-5.2 model_ratio = %v, want 0.875", got)
	}
	if got := modelRatioMap["claude-opus-4-5"]; got != 2.5 {
		t.Fatalf("claude-opus-4-5 model_ratio = %v, want 2.5", got)
	}
	if got := modelRatioMap["gpt-4o"]; got != 1.25 {
		t.Fatalf("gpt-4o model_ratio = %v, want 1.25", got)
	}
	if got := modelRatioMap["gpt-4o:extended"]; got != 3.0 {
		t.Fatalf("gpt-4o:extended model_ratio = %v, want 3", got)
	}
	if got := modelRatioMap["gemma-4-26b-a4b-it:free"]; got != 0.0 {
		t.Fatalf("gemma-4-26b-a4b-it:free model_ratio = %v, want 0", got)
	}

	completionRatioMap, ok := converted["completion_ratio"].(map[string]any)
	if !ok {
		t.Fatalf("completion_ratio missing or invalid: %#v", converted["completion_ratio"])
	}
	if got := completionRatioMap["gpt-5.2"]; got != 8.0 {
		t.Fatalf("gpt-5.2 completion_ratio = %v, want 8", got)
	}
	if got := completionRatioMap["claude-opus-4-5"]; got != 5.0 {
		t.Fatalf("claude-opus-4-5 completion_ratio = %v, want 5", got)
	}
	if got := completionRatioMap["gpt-4o"]; got != 4.0 {
		t.Fatalf("gpt-4o completion_ratio = %v, want 4", got)
	}
	if got := completionRatioMap["gpt-4o:extended"]; got != 3.0 {
		t.Fatalf("gpt-4o:extended completion_ratio = %v, want 3", got)
	}
	if got := completionRatioMap["gemma-4-26b-a4b-it:free"]; got != 0.0 {
		t.Fatalf("gemma-4-26b-a4b-it:free completion_ratio = %v, want 0", got)
	}
	if got := completionRatioMap["default-completion-and-cache-model"]; got != 1.0 {
		t.Fatalf("default-completion-and-cache-model completion_ratio = %v, want 1", got)
	}
	if got := completionRatioMap["free-missing-fields"]; got != 0.0 {
		t.Fatalf("free-missing-fields completion_ratio = %v, want 0", got)
	}

	cacheRatioMap, ok := converted["cache_ratio"].(map[string]any)
	if !ok {
		t.Fatalf("cache_ratio missing or invalid: %#v", converted["cache_ratio"])
	}
	if got := cacheRatioMap["gpt-5.2"]; got != 0.1 {
		t.Fatalf("gpt-5.2 cache_ratio = %v, want 0.1", got)
	}
	if got := cacheRatioMap["claude-opus-4-5"]; got != 0.1 {
		t.Fatalf("claude-opus-4-5 cache_ratio = %v, want 0.1", got)
	}
	if got := cacheRatioMap["default-cache-model"]; got != 1.0 {
		t.Fatalf("default-cache-model cache_ratio = %v, want 1", got)
	}
	if got := cacheRatioMap["default-completion-and-cache-model"]; got != 1.0 {
		t.Fatalf("default-completion-and-cache-model cache_ratio = %v, want 1", got)
	}
	if got := cacheRatioMap["free-missing-fields"]; got != 0.0 {
		t.Fatalf("free-missing-fields cache_ratio = %v, want 0", got)
	}

	createCacheRatioMap, ok := converted["create_cache_ratio"].(map[string]any)
	if !ok {
		t.Fatalf("create_cache_ratio missing or invalid: %#v", converted["create_cache_ratio"])
	}
	if got := createCacheRatioMap["gpt-5.2"]; got != 1.0 {
		t.Fatalf("gpt-5.2 create_cache_ratio = %v, want 1", got)
	}
	if got := createCacheRatioMap["claude-opus-4-5"]; got != 1.25 {
		t.Fatalf("claude-opus-4-5 create_cache_ratio = %v, want 1.25", got)
	}
	if got := createCacheRatioMap["gemma-4-26b-a4b-it:free"]; got != 0.0 {
		t.Fatalf("gemma-4-26b-a4b-it:free create_cache_ratio = %v, want 0", got)
	}
	if got := createCacheRatioMap["default-cache-model"]; got != 1.25 {
		t.Fatalf("default-cache-model create_cache_ratio = %v, want 1.25", got)
	}
	if got := createCacheRatioMap["default-completion-and-cache-model"]; got != 1.25 {
		t.Fatalf("default-completion-and-cache-model create_cache_ratio = %v, want 1.25", got)
	}
	if got := createCacheRatioMap["free-missing-fields"]; got != 0.0 {
		t.Fatalf("free-missing-fields create_cache_ratio = %v, want 0", got)
	}

	modelPriceMap, ok := converted["model_price"].(map[string]any)
	if !ok {
		t.Fatalf("model_price missing or invalid: %#v", converted["model_price"])
	}
	if got := modelPriceMap["per-call-model"]; got != 0.04 {
		t.Fatalf("per-call-model model_price = %v, want 0.04", got)
	}
	if _, exists := modelPriceMap["audio-only-model"]; exists {
		t.Fatalf("audio-only-model should have been ignored, got %#v", modelPriceMap["audio-only-model"])
	}
}

func TestResolveUpstreamBearerToken(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	original := common.OptionMap
	common.OptionMap = map[string]string{
		openRouterTokenOption: "  stored-token  ",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = original
		common.OptionMapRWMutex.Unlock()
	})

	t.Run("request token overrides stored token", func(t *testing.T) {
		token := resolveUpstreamBearerToken(dto.UpstreamDTO{
			SourceType:  syncSourceTypeOpenRouter,
			BearerToken: " request-token ",
		})
		if token != "request-token" {
			t.Fatalf("resolveUpstreamBearerToken() = %q, want %q", token, "request-token")
		}
	})

	t.Run("openrouter falls back to stored token", func(t *testing.T) {
		token := resolveUpstreamBearerToken(dto.UpstreamDTO{
			SourceType: syncSourceTypeOpenRouter,
		})
		if token != "stored-token" {
			t.Fatalf("resolveUpstreamBearerToken() = %q, want %q", token, "stored-token")
		}
	})

	t.Run("non openrouter does not use stored token", func(t *testing.T) {
		token := resolveUpstreamBearerToken(dto.UpstreamDTO{})
		if token != "" {
			t.Fatalf("resolveUpstreamBearerToken() = %q, want empty", token)
		}
	})
}
