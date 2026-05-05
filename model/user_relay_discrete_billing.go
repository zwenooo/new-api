package model

import "fmt"

type RelayDiscreteBillingState struct {
	UserId           int   `json:"user_id"`
	PaygQuota        int   `json:"payg_quota"`
	PaygGroups       []int `json:"payg_groups"`
	PayRequestQuota  int   `json:"pay_request_quota"`
	PayRequestGroups []int `json:"pay_request_groups"`
	PayTokenQuota    int   `json:"pay_token_quota"`
	PayTokenGroups   []int `json:"pay_token_groups"`
}

func parseUserSnapshotGroupIDs(value JSONValue) ([]int, error) {
	if len(value) == 0 {
		return nil, nil
	}
	if ids, err := ParseGroupIDsJSON(value); err == nil && len(ids) > 0 {
		return ids, nil
	}
	codes, err := ParseGroupNamesJSON(value)
	if err != nil {
		return nil, err
	}
	if len(codes) == 0 {
		return nil, nil
	}
	ids, _, err := existingLegacyGroupIDsFromCodes(nil, codes)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func GetUserRelayDiscreteBillingState(userId int) (*RelayDiscreteBillingState, error) {
	if userId <= 0 {
		return nil, fmt.Errorf("userId 无效")
	}

	var user User
	if err := DB.Select(
		"id",
		"payg_quota",
		"payg_allowed_groups",
		"pay_request_quota",
		"pay_request_allowed_groups",
		"pay_token_quota",
		"pay_token_allowed_groups",
	).Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}

	paygGroups, err := parseUserSnapshotGroupIDs(user.PayAsYouGoAllowedGroups)
	if err != nil {
		return nil, err
	}
	payRequestGroups, err := parseUserSnapshotGroupIDs(user.PayRequestAllowedGroups)
	if err != nil {
		return nil, err
	}
	payTokenGroups, err := parseUserSnapshotGroupIDs(user.PayTokenAllowedGroups)
	if err != nil {
		return nil, err
	}

	return &RelayDiscreteBillingState{
		UserId:           user.Id,
		PaygQuota:        user.PayAsYouGoQuota,
		PaygGroups:       paygGroups,
		PayRequestQuota:  user.PayRequestQuota,
		PayRequestGroups: payRequestGroups,
		PayTokenQuota:    user.PayTokenQuota,
		PayTokenGroups:   payTokenGroups,
	}, nil
}
