package codexauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type JWTClaims struct {
	Exp        int64            `json:"exp"`
	OpenAIAuth OpenAIAuthClaims `json:"https://api.openai.com/auth"`
}

type OpenAIAuthClaims struct {
	ChatgptAccountID string `json:"chatgpt_account_id"`
	ChatgptPlanType  string `json:"chatgpt_plan_type"`
}

func ParseJWTClaims(token string) (*JWTClaims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("empty jwt token")
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid jwt token format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("decode jwt payload: %w", err)
		}
	}
	var claims JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal jwt payload: %w", err)
	}
	return &claims, nil
}

func ExtractPlanType(token string) string {
	claims, err := ParseJWTClaims(token)
	if err != nil || claims == nil {
		return ""
	}
	return strings.TrimSpace(claims.OpenAIAuth.ChatgptPlanType)
}
