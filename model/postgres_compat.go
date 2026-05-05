package model

import (
	"one-api/common"

	"gorm.io/gorm"
)

func postgresCompatPreMigrationSQL() []string {
	return []string{
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'longtext') THEN CREATE DOMAIN longtext AS text; END IF; END $$;`,
	}
}

func ensurePostgresCompatTypes(db *gorm.DB) error {
	if db == nil || !common.UsingPostgreSQL {
		return nil
	}
	for _, stmt := range postgresCompatPreMigrationSQL() {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
