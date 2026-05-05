package model

import (
	"fmt"
	"one-api/common"
)

func jsonColumnIsEmptyCondition(column string) string {
	switch {
	case common.UsingMySQL:
		return fmt.Sprintf("(%s IS NULL OR JSON_LENGTH(%s) = 0)", column, column)
	case common.UsingPostgreSQL:
		return fmt.Sprintf("(%s IS NULL OR %s::text = 'null' OR %s::text = '[]')", column, column, column)
	default:
		return fmt.Sprintf("(%s IS NULL OR %s = '' OR %s = 'null' OR %s = '[]')", column, column, column, column)
	}
}

func jsonColumnIsNotEmptyCondition(column string) string {
	switch {
	case common.UsingMySQL:
		return fmt.Sprintf("(%s IS NOT NULL AND JSON_LENGTH(%s) > 0)", column, column)
	case common.UsingPostgreSQL:
		return fmt.Sprintf("(%s IS NOT NULL AND %s::text <> 'null' AND %s::text <> '[]')", column, column, column)
	default:
		return fmt.Sprintf("(%s IS NOT NULL AND %s <> '' AND %s <> 'null' AND %s <> '[]')", column, column, column, column)
	}
}
