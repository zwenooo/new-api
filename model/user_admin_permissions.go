package model

import (
	"bytes"
	"one-api/common"
	"one-api/constant"
)

type AdminPermissions struct {
	ProductManagement bool `json:"product_management,omitempty"`
	Order             bool `json:"order,omitempty"`
}

func NormalizeAdminPermissions(perms AdminPermissions) AdminPermissions {
	return AdminPermissions{
		ProductManagement: perms.ProductManagement,
		Order:             perms.Order,
	}
}

func (perms AdminPermissions) Has(module string) bool {
	switch module {
	case constant.AdminModuleProductManagement:
		return perms.ProductManagement
	case constant.AdminModuleOrder:
		return perms.Order
	default:
		return false
	}
}

func (perms AdminPermissions) HasAny() bool {
	return perms.ProductManagement || perms.Order
}

func ParseAdminPermissionsJSON(raw JSONValue) (AdminPermissions, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return AdminPermissions{}, nil
	}

	var perms AdminPermissions
	if err := common.Unmarshal(trimmed, &perms); err != nil {
		return AdminPermissions{}, err
	}
	return NormalizeAdminPermissions(perms), nil
}

func MarshalAdminPermissionsJSON(perms AdminPermissions) (JSONValue, error) {
	normalized := NormalizeAdminPermissions(perms)
	b, err := common.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return JSONValue(b), nil
}

func EmptyAdminPermissionsJSON() JSONValue {
	raw, err := MarshalAdminPermissionsJSON(AdminPermissions{})
	if err != nil {
		return JSONValue([]byte("{}"))
	}
	return raw
}

func HasAdminModulePermission(role int, perms AdminPermissions, module string) bool {
	if role >= common.RoleRootUser {
		return true
	}
	if role < common.RoleAdminUser {
		return false
	}
	return perms.Has(module)
}

func (user *User) GetAdminPermissions() AdminPermissions {
	if user == nil {
		return AdminPermissions{}
	}
	perms, err := ParseAdminPermissionsJSON(user.AdminPermissions)
	if err != nil {
		common.SysLog("failed to parse user admin permissions: " + err.Error())
		return AdminPermissions{}
	}
	return perms
}

func (user *User) SetAdminPermissions(perms AdminPermissions) error {
	if user == nil {
		return nil
	}
	raw, err := MarshalAdminPermissionsJSON(perms)
	if err != nil {
		return err
	}
	user.AdminPermissions = raw
	return nil
}

func (user *UserBase) GetAdminPermissions() AdminPermissions {
	if user == nil {
		return AdminPermissions{}
	}
	perms, err := ParseAdminPermissionsJSON(user.AdminPermissions)
	if err != nil {
		common.SysLog("failed to parse cached user admin permissions: " + err.Error())
		return AdminPermissions{}
	}
	return perms
}
