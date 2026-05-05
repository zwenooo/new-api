package model

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func lockForUpdate(tx *gorm.DB) *gorm.DB {
	if tx == nil {
		return tx
	}
	if strings.EqualFold(tx.Dialector.Name(), "sqlite") {
		return tx
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}

func supportsForUpdate(tx *gorm.DB) bool {
	if tx == nil {
		return false
	}
	return !strings.EqualFold(tx.Dialector.Name(), "sqlite")
}
