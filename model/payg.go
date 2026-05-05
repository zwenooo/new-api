package model

import "errors"

func GetUserPaygInfo(userId int) (paygQuota int, allowedGroupIDs []int, err error) {
	if userId <= 0 {
		return 0, nil, errors.New("userId 无效")
	}
	var user User
	if err := DB.Select("payg_quota").Where("id = ?", userId).First(&user).Error; err != nil {
		return 0, nil, err
	}
	_, groups, err := GetUserPaygBalanceInfoTx(DB, userId)
	if err != nil {
		return 0, nil, err
	}
	return user.PayAsYouGoQuota, groups, nil
}
