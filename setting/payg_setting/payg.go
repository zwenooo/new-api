package payg_setting

import (
	"errors"
	"one-api/setting/config"
	"sort"
	"strings"
	"unicode/utf8"
)

type PaygProduct struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Archived    bool   `json:"archived"`
	SortOrder   int    `json:"sort_order"`
	// Stock is the remaining inventory for this product.
	// nil means unlimited; 0 means sold out.
	Stock *int `json:"stock"`
	// AllowedGroupIds is the source of truth. allowed_groups is legacy-only compatibility.
	AllowedGroupIds []int    `json:"allowed_group_ids"`
	AllowedGroups   []string `json:"allowed_groups,omitempty"`
}

type PayRequestProduct struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Archived    bool   `json:"archived"`
	SortOrder   int    `json:"sort_order"`
	// Stock is the remaining inventory for this product.
	// nil means unlimited; 0 means sold out.
	Stock *int `json:"stock"`
	// AllowedGroupIds is the source of truth. allowed_groups is legacy-only compatibility.
	AllowedGroupIds []int    `json:"allowed_group_ids"`
	AllowedGroups   []string `json:"allowed_groups,omitempty"`
}

type PayTokenProduct struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Archived    bool   `json:"archived"`
	SortOrder   int    `json:"sort_order"`
	// Stock is the remaining inventory for this product.
	// nil means unlimited; 0 means sold out.
	Stock *int `json:"stock"`
	// AllowedGroupIds is the source of truth. allowed_groups is legacy-only compatibility.
	AllowedGroupIds []int    `json:"allowed_group_ids"`
	AllowedGroups   []string `json:"allowed_groups,omitempty"`
}

// PaygSettings controls pay-as-you-go credit conversion.
// credit_usd_per_cny means: credited USD quota = RMB(yuan) * credit_usd_per_cny.
type PaygSettings struct {
	Enabled         bool    `json:"enabled"`
	Description     string  `json:"description"`
	CreditUsdPerCny float64 `json:"credit_usd_per_cny"`
	// CreditRequestsPerCny means: credited request count = RMB(yuan) * credit_requests_per_cny.
	// 0 means disabled / not configured.
	CreditRequestsPerCny int `json:"credit_requests_per_cny"`
	// CreditTokensPerCny means: credited tokens = RMB(yuan) * credit_tokens_per_cny.
	// 0 means disabled / not configured.
	CreditTokensPerCny int                 `json:"credit_tokens_per_cny"`
	Products           []PaygProduct       `json:"products"`
	PayRequestProducts []PayRequestProduct `json:"pay_request_products"`
	PayTokenProducts   []PayTokenProduct   `json:"pay_token_products"`
}

var defaultPaygSettings = PaygSettings{
	Enabled:              true,
	Description:          "",
	CreditUsdPerCny:      20,
	CreditRequestsPerCny: 0,
	CreditTokensPerCny:   0,
	Products:             []PaygProduct{},
	PayRequestProducts:   []PayRequestProduct{},
	PayTokenProducts:     []PayTokenProduct{},
}

var paygSettings = defaultPaygSettings

func init() {
	config.GlobalConfig.Register("payg", &paygSettings)
}

func GetPaygSettings() *PaygSettings {
	return &paygSettings
}

func normalizeGroups(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, v := range raw {
		name := strings.TrimSpace(v)
		if name == "" {
			continue
		}
		if utf8.RuneCountInString(name) > 64 {
			return nil, errors.New("分组名称过长")
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func normalizeGroupIDs(raw []int) ([]int, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[int]struct{}, len(raw))
	out := make([]int, 0, len(raw))
	for _, v := range raw {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Ints(out)
	return out, nil
}

func NormalizePaygProducts(products []PaygProduct) ([]PaygProduct, error) {
	if products == nil {
		return nil, nil
	}

	seenIds := make(map[int]struct{}, len(products))
	out := make([]PaygProduct, 0, len(products))
	for _, raw := range products {
		p := raw
		p.Name = strings.TrimSpace(p.Name)
		p.Description = strings.TrimSpace(p.Description)

		if p.Id <= 0 {
			return nil, errors.New("按量付费商品 id 无效")
		}
		if _, ok := seenIds[p.Id]; ok {
			return nil, errors.New("按量付费商品 id 重复")
		}
		seenIds[p.Id] = struct{}{}

		nameLen := utf8.RuneCountInString(p.Name)
		if nameLen == 0 || nameLen > 64 {
			return nil, errors.New("按量付费商品名称长度必须在 1-64 之间")
		}
		if utf8.RuneCountInString(p.Description) > 2048 {
			return nil, errors.New("按量付费商品描述长度不能超过 2048")
		}
		if p.SortOrder < 0 {
			return nil, errors.New("按量付费商品 sort_order 不能小于 0")
		}
		if p.Stock != nil && *p.Stock < 0 {
			return nil, errors.New("按量付费商品库存不能小于 0")
		}
		if p.Archived {
			p.Enabled = false
		}

		if len(p.AllowedGroupIds) > 0 {
			normalizedIDs, err := normalizeGroupIDs(p.AllowedGroupIds)
			if err != nil {
				return nil, err
			}
			if len(normalizedIDs) == 0 {
				return nil, errors.New("按量付费商品可用分组不能为空")
			}
			p.AllowedGroupIds = normalizedIDs
			p.AllowedGroups = nil
		} else {
			// Legacy-only compatibility
			normalizedGroups, err := normalizeGroups(p.AllowedGroups)
			if err != nil {
				return nil, err
			}
			if len(normalizedGroups) == 0 {
				return nil, errors.New("按量付费商品可用分组不能为空")
			}
			p.AllowedGroups = normalizedGroups
		}
		out = append(out, p)
	}
	return out, nil
}

func FindPaygProductByID(id int) (*PaygProduct, bool) {
	if id <= 0 {
		return nil, false
	}
	products := paygSettings.Products
	for _, p := range products {
		if p.Id == id {
			cp := p
			return &cp, true
		}
	}
	return nil, false
}

func NormalizePayRequestProducts(products []PayRequestProduct) ([]PayRequestProduct, error) {
	if products == nil {
		return nil, nil
	}

	seenIds := make(map[int]struct{}, len(products))
	out := make([]PayRequestProduct, 0, len(products))
	for _, raw := range products {
		p := raw
		p.Name = strings.TrimSpace(p.Name)
		p.Description = strings.TrimSpace(p.Description)

		if p.Id <= 0 {
			return nil, errors.New("按次付费商品 id 无效")
		}
		if _, ok := seenIds[p.Id]; ok {
			return nil, errors.New("按次付费商品 id 重复")
		}
		seenIds[p.Id] = struct{}{}

		nameLen := utf8.RuneCountInString(p.Name)
		if nameLen == 0 || nameLen > 64 {
			return nil, errors.New("按次付费商品名称长度必须在 1-64 之间")
		}
		if utf8.RuneCountInString(p.Description) > 2048 {
			return nil, errors.New("按次付费商品描述长度不能超过 2048")
		}
		if p.SortOrder < 0 {
			return nil, errors.New("按次付费商品 sort_order 不能小于 0")
		}
		if p.Stock != nil && *p.Stock < 0 {
			return nil, errors.New("按次付费商品库存不能小于 0")
		}
		if p.Archived {
			p.Enabled = false
		}

		if len(p.AllowedGroupIds) > 0 {
			normalizedIDs, err := normalizeGroupIDs(p.AllowedGroupIds)
			if err != nil {
				return nil, err
			}
			if len(normalizedIDs) == 0 {
				return nil, errors.New("按次付费商品可用分组不能为空")
			}
			p.AllowedGroupIds = normalizedIDs
			p.AllowedGroups = nil
		} else {
			// Legacy-only compatibility
			normalizedGroups, err := normalizeGroups(p.AllowedGroups)
			if err != nil {
				return nil, err
			}
			if len(normalizedGroups) == 0 {
				return nil, errors.New("按次付费商品可用分组不能为空")
			}
			p.AllowedGroups = normalizedGroups
		}
		out = append(out, p)
	}
	return out, nil
}

func FindPayRequestProductByID(id int) (*PayRequestProduct, bool) {
	if id <= 0 {
		return nil, false
	}
	products := paygSettings.PayRequestProducts
	for _, p := range products {
		if p.Id == id {
			cp := p
			return &cp, true
		}
	}
	return nil, false
}

func NormalizePayTokenProducts(products []PayTokenProduct) ([]PayTokenProduct, error) {
	if products == nil {
		return nil, nil
	}

	seenIds := make(map[int]struct{}, len(products))
	out := make([]PayTokenProduct, 0, len(products))
	for _, raw := range products {
		p := raw
		p.Name = strings.TrimSpace(p.Name)
		p.Description = strings.TrimSpace(p.Description)

		if p.Id <= 0 {
			return nil, errors.New("按token付费商品 id 无效")
		}
		if _, ok := seenIds[p.Id]; ok {
			return nil, errors.New("按token付费商品 id 重复")
		}
		seenIds[p.Id] = struct{}{}

		nameLen := utf8.RuneCountInString(p.Name)
		if nameLen == 0 || nameLen > 64 {
			return nil, errors.New("按token付费商品名称长度必须在 1-64 之间")
		}
		if utf8.RuneCountInString(p.Description) > 2048 {
			return nil, errors.New("按token付费商品描述长度不能超过 2048")
		}
		if p.SortOrder < 0 {
			return nil, errors.New("按token付费商品 sort_order 不能小于 0")
		}
		if p.Stock != nil && *p.Stock < 0 {
			return nil, errors.New("按token付费商品库存不能小于 0")
		}
		if p.Archived {
			p.Enabled = false
		}

		if len(p.AllowedGroupIds) > 0 {
			normalizedIDs, err := normalizeGroupIDs(p.AllowedGroupIds)
			if err != nil {
				return nil, err
			}
			if len(normalizedIDs) == 0 {
				return nil, errors.New("按token付费商品可用分组不能为空")
			}
			p.AllowedGroupIds = normalizedIDs
			p.AllowedGroups = nil
		} else {
			// Legacy-only compatibility
			normalizedGroups, err := normalizeGroups(p.AllowedGroups)
			if err != nil {
				return nil, err
			}
			if len(normalizedGroups) == 0 {
				return nil, errors.New("按token付费商品可用分组不能为空")
			}
			p.AllowedGroups = normalizedGroups
		}
		out = append(out, p)
	}
	return out, nil
}

func FindPayTokenProductByID(id int) (*PayTokenProduct, bool) {
	if id <= 0 {
		return nil, false
	}
	products := paygSettings.PayTokenProducts
	for _, p := range products {
		if p.Id == id {
			cp := p
			return &cp, true
		}
	}
	return nil, false
}
