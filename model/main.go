package model

import (
	"fmt"
	"log"
	"one-api/common"
	"one-api/constant"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var commonGroupCol string
var commonKeyCol string
var commonTrueVal string
var commonFalseVal string

var logKeyCol string
var logGroupCol string

// shouldPrepareStmt returns whether gorm should enable PrepareStmt for the given dialect.
//
// Background:
//   - gorm.Config.PrepareStmt caches *sql.Stmt for each distinct SQL string.
//   - On MySQL, server-side prepared statements are counted in `max_prepared_stmt_count` and
//     are per-connection. With a moderate connection pool and many distinct SQL shapes
//     (especially varying `IN (...)` placeholders), this can exhaust MySQL's prepared statement
//     capacity and lead to severe stalls.
//
// Default policy:
//   - MySQL: disabled by default (safer for long-running/high-QPS services).
//   - SQLite/PostgreSQL: enabled by default (usually beneficial).
//
// Override:
//   - Set env SQL_PREPARE_STMT=true/false to force enable/disable.
func shouldPrepareStmt(dialect string) bool {
	raw := strings.TrimSpace(os.Getenv("SQL_PREPARE_STMT"))
	if raw != "" {
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		default:
			// Invalid value: fall through to dialect defaults.
		}
	}
	// Dialect defaults
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case common.DatabaseTypeMySQL:
		return false
	default:
		return true
	}
}

func initCol() {
	// init common column names
	if common.UsingPostgreSQL {
		commonGroupCol = `"group"`
		commonKeyCol = `"key"`
		commonTrueVal = "true"
		commonFalseVal = "false"
	} else {
		commonGroupCol = "`group`"
		commonKeyCol = "`key`"
		commonTrueVal = "1"
		commonFalseVal = "0"
	}
	if os.Getenv("LOG_SQL_DSN") != "" {
		switch common.LogSqlType {
		case common.DatabaseTypePostgreSQL:
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		default:
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	} else {
		// LOG_SQL_DSN 为空时，日志数据库与主数据库相同
		if common.UsingPostgreSQL {
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		} else {
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	}
	// log sql type and database type
	//common.SysLog("Using Log SQL Type: " + common.LogSqlType)
}

var DB *gorm.DB

var LOG_DB *gorm.DB

func createRootAccountIfNeed() error {
	var user User
	//if user.Status != common.UserStatusEnabled {
	if err := DB.First(&user).Error; err != nil {
		common.SysLog("no user exists, create a root user for you: username is root, password is 123456")
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return err
		}
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		DB.Create(&rootUser)
	}
	return nil
}

func CheckSetup() {
	setup := GetSetup()
	if setup == nil {
		// No setup record exists, check if we have a root user
		if RootUserExists() {
			common.SysLog("system is not initialized, but root user exists")
			// Create setup record
			newSetup := Setup{
				Version:       common.Version,
				InitializedAt: time.Now().Unix(),
			}
			err := DB.Create(&newSetup).Error
			if err != nil {
				common.SysLog("failed to create setup record: " + err.Error())
			}
			constant.Setup = true
		} else {
			common.SysLog("system is not initialized and no root user exists")
			constant.Setup = false
		}
	} else {
		// Setup record exists, system is initialized
		common.SysLog("system is already initialized at: " + time.Unix(setup.InitializedAt, 0).String())
		constant.Setup = true
	}
}

func chooseDB(envName string, isLog bool) (*gorm.DB, error) {
	defer func() {
		initCol()
	}()
	dsn := os.Getenv(envName)
	if dsn != "" {
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			// Use PostgreSQL
			common.SysLog("using PostgreSQL as database")
			if !isLog {
				common.UsingPostgreSQL = true
			} else {
				common.LogSqlType = common.DatabaseTypePostgreSQL
			}
			return gorm.Open(postgres.New(postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: true, // disables implicit prepared statement usage
			}), &gorm.Config{
				PrepareStmt: shouldPrepareStmt(common.DatabaseTypePostgreSQL), // precompile SQL
			})
		}
		if strings.HasPrefix(dsn, "local") {
			common.SysLog("SQL_DSN not set, using SQLite as database")
			if !isLog {
				common.UsingSQLite = true
			} else {
				common.LogSqlType = common.DatabaseTypeSQLite
			}
			return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
				PrepareStmt: shouldPrepareStmt(common.DatabaseTypeSQLite), // precompile SQL
			})
		}
		// Use MySQL
		common.SysLog("using MySQL as database")
		// check parseTime
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		if !isLog {
			common.UsingMySQL = true
		} else {
			common.LogSqlType = common.DatabaseTypeMySQL
		}
		return gorm.Open(mysql.Open(dsn), &gorm.Config{
			PrepareStmt: shouldPrepareStmt(common.DatabaseTypeMySQL), // precompile SQL
		})
	}
	// Use SQLite
	common.SysLog("SQL_DSN not set, using SQLite as database")
	common.UsingSQLite = true
	return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
		PrepareStmt: shouldPrepareStmt(common.DatabaseTypeSQLite), // precompile SQL
	})
}

func InitDB() (err error) {
	db, err := chooseDB("SQL_DSN", false)
	if err == nil {
		if common.GormSQLLogEnabled {
			db = db.Debug()
		}
		DB = db
		// MySQL charset/collation startup check: ensure Chinese-capable charset
		if common.UsingMySQL {
			if err := checkMySQLChineseSupport(DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		// Connection pool defaults should be conservative for MySQL (avoid overload).
		// docker-compose.yml already sets these env vars; these are fallbacks for other launch modes.
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 20))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 100))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 300)))

		if !common.IsMasterNode {
			return nil
		}
		if common.UsingMySQL {
			//_, _ = sqlDB.Exec("ALTER TABLE channels MODIFY model_mapping TEXT;") // TODO: delete this line when most users have upgraded
		}
		common.SysLog("database migration started")
		err = migrateDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func InitLogDB() (err error) {
	if os.Getenv("LOG_SQL_DSN") == "" {
		switch {
		case common.UsingPostgreSQL:
			common.LogSqlType = common.DatabaseTypePostgreSQL
		case common.UsingMySQL:
			common.LogSqlType = common.DatabaseTypeMySQL
		default:
			common.LogSqlType = common.DatabaseTypeSQLite
		}
		LOG_DB = DB
		return
	}
	db, err := chooseDB("LOG_SQL_DSN", true)
	if err == nil {
		if common.GormSQLLogEnabled {
			db = db.Debug()
		}
		LOG_DB = db
		// If log DB is MySQL, also ensure Chinese-capable charset
		if common.LogSqlType == common.DatabaseTypeMySQL {
			if err := checkMySQLChineseSupport(LOG_DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := LOG_DB.DB()
		if err != nil {
			return err
		}
		// Keep log DB pool defaults consistent with main DB defaults.
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 20))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 100))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 300)))

		if !common.IsMasterNode {
			return nil
		}
		common.SysLog("database migration started")
		err = migrateLOGDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func migrateDB() error {
	runStep := func(name string, fn func() error) error {
		if err := fn(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
	runStepTx := func(name string, fn func(tx *gorm.DB) error) error {
		return runStep(name, func() error {
			return DB.Transaction(func(tx *gorm.DB) error {
				return fn(tx)
			})
		})
	}

	if err := ensurePostgresCompatTypes(DB); err != nil {
		return err
	}
	hasPresetEnabled := DB.Migrator().HasColumn(&RedemptionPreset{}, "enabled")
	hasPresetArchived := DB.Migrator().HasColumn(&RedemptionPreset{}, "archived")
	hasPresetMultiQuantityEnabled := DB.Migrator().HasColumn(&RedemptionPreset{}, "multi_quantity_enabled")
	hasPresetMultiQuantityDeferOnly := DB.Migrator().HasColumn(&RedemptionPreset{}, "multi_quantity_defer_only")
	hasPresetRevisionArchived := DB.Migrator().HasColumn(&RedemptionPresetRevision{}, "archived")
	hasPaygProductArchived := DB.Migrator().HasColumn(&PaygProduct{}, "archived")
	hasPayRequestProductArchived := DB.Migrator().HasColumn(&PayRequestProduct{}, "archived")
	hasPayTokenProductArchived := DB.Migrator().HasColumn(&PayTokenProduct{}, "archived")
	hasPaygProductRevisionArchived := DB.Migrator().HasColumn(&PaygProductRevision{}, "archived")
	hasPayRequestProductRevisionArchived := DB.Migrator().HasColumn(&PayRequestProductRevision{}, "archived")
	hasPayTokenProductRevisionArchived := DB.Migrator().HasColumn(&PayTokenProductRevision{}, "archived")
	hasSubscriptionOrderQuantity := DB.Migrator().HasColumn(&SubscriptionOrder{}, "quantity")
	hasUserSubscriptionCredited := DB.Migrator().HasColumn(&UserSubscription{}, "credited")
	hasUserSubscriptionBillingUnit := DB.Migrator().HasColumn(&UserSubscription{}, "billing_unit")
	err := DB.AutoMigrate(
		&UserGroup{},
		&Group{},
		&ChannelGroup{},
		&ChannelBackupGroup{},
		&TokenAllowedGroup{},
		&SubscriptionProductGroup{},
		&PaygProductRevision{},
		&PaygProductRevisionGroup{},
		&PayRequestProductRevision{},
		&PayRequestProductRevisionGroup{},
		&PayTokenProductRevision{},
		&PayTokenProductRevisionGroup{},
		&RedemptionPresetRevision{},
		&RedemptionPresetRevisionGroup{},
		&RedemptionPresetRevisionGroupDailyLimit{},
		&SubscriptionProductGroupDailyLimit{},
		&RedemptionGroupDailyLimit{},
		&UserSubscriptionGroupDailyLimit{},
		&UserSubscriptionGroupDailyUsage{},
		&PaygProduct{},
		&PaygProductGroup{},
		&PayRequestProduct{},
		&PayRequestProductGroup{},
		&PayTokenProduct{},
		&PayTokenProductGroup{},
		&UserSubscriptionGroup{},
		&UserRequestSubscriptionGroup{},
		&PricingProfile{},
		&PricingProfileGroupFactor{},
		&UserGroupPriceOverride{},
		&Channel{},
		&ChannelRequestDailyStat{},
		&UserRequestDailyStat{},
		&ChannelUserBinding{},
		&Token{},
		&User{},
		&BalanceRecord{},
		&Option{},
		&ClawBoxPortableRelease{},
		&Redemption{},
		&RedemptionPreset{},
		&Ability{},
		&Log{},
		&RequestTraceSession{},
		&RequestTraceNode{},
		&Midjourney{},
		&TopUp{},
		&QuotaData{},
		&Task{},
		&TaskSubmitRepair{},
		&Model{},
		&Vendor{},
		&PrefillGroup{},
		&Setup{},
		&TwoFA{},
		&TwoFABackupCode{},
		&UserSubscription{},
		&UserRequestSubscription{},
		&UserSubscriptionPresetRevisionBinding{},
		&UserRequestSubscriptionPresetRevisionBinding{},
		&SubscriptionPlan{},
		&SubscriptionOrder{},
		&PaygOrder{},
		&PayRequestOrder{},
		&PayTokenOrder{},
		&PaygUserBalance{},
		&PayRequestUserBalance{},
		&PayTokenUserBalance{},
		&UserMembership{},
		&ClawBoxDeviceSession{},
		&ClawBoxPortableBinding{},
	)
	if err != nil {
		return err
	}
	if err := dropUserSubscriptionsCreditedOnlyIndexIfNeeded(DB); err != nil {
		return err
	}
	if !hasUserSubscriptionCredited {
		// When introducing the credited flag, all historical subscriptions should be treated as credited,
		// since legacy behavior always credited subscriptions immediately.
		if err := DB.Model(&UserSubscription{}).Where("credited = ?", false).Update("credited", true).Error; err != nil {
			return err
		}
	}
	// service_status_bucket_stats lives in LOG_DB; when LOG_DB == DB, migrate here as well.
	if os.Getenv("LOG_SQL_DSN") == "" {
		if err := DB.AutoMigrate(&ServiceStatusBucketStat{}, &ServiceStatusRequestState{}, &ServiceStatusBucketStatsMeta{}); err != nil {
			return err
		}
	}
	if !hasPresetEnabled {
		if err := DB.Model(&RedemptionPreset{}).Where("id > 0").Update("enabled", true).Error; err != nil {
			return err
		}
	}
	if !hasPresetArchived {
		if err := DB.Model(&RedemptionPreset{}).Where("id > 0").Update("archived", false).Error; err != nil {
			return err
		}
	}
	if !hasPresetRevisionArchived {
		if err := DB.Model(&RedemptionPresetRevision{}).Where("id > 0").Update("archived", false).Error; err != nil {
			return err
		}
	}
	if !hasPaygProductArchived {
		if err := DB.Model(&PaygProduct{}).Where("id > 0").Update("archived", false).Error; err != nil {
			return err
		}
	}
	if !hasPayRequestProductArchived {
		if err := DB.Model(&PayRequestProduct{}).Where("id > 0").Update("archived", false).Error; err != nil {
			return err
		}
	}
	if !hasPayTokenProductArchived {
		if err := DB.Model(&PayTokenProduct{}).Where("id > 0").Update("archived", false).Error; err != nil {
			return err
		}
	}
	if !hasPaygProductRevisionArchived {
		if err := DB.Model(&PaygProductRevision{}).Where("id > 0").Update("archived", false).Error; err != nil {
			return err
		}
	}
	if !hasPayRequestProductRevisionArchived {
		if err := DB.Model(&PayRequestProductRevision{}).Where("id > 0").Update("archived", false).Error; err != nil {
			return err
		}
	}
	if !hasPayTokenProductRevisionArchived {
		if err := DB.Model(&PayTokenProductRevision{}).Where("id > 0").Update("archived", false).Error; err != nil {
			return err
		}
	}
	if !hasPresetMultiQuantityEnabled {
		// keep default disabled for existing presets
		if err := DB.Model(&RedemptionPreset{}).Where("id > 0").Update("multi_quantity_enabled", false).Error; err != nil {
			return err
		}
	}
	if !hasPresetMultiQuantityDeferOnly {
		// preserve historical behavior: multi-quantity purchases are defer-only by default
		if err := DB.Model(&RedemptionPreset{}).Where("id > 0").Update("multi_quantity_defer_only", true).Error; err != nil {
			return err
		}
	}
	if !hasSubscriptionOrderQuantity {
		// backfill legacy rows (and avoid future failures) to ensure quantity >= 1
		if err := DB.Model(&SubscriptionOrder{}).
			Where("quantity IS NULL OR quantity <= 0").
			Update("quantity", 1).Error; err != nil {
			return err
		}
	}
	if !hasUserSubscriptionBillingUnit {
		if err := DB.Model(&UserSubscription{}).
			Where("billing_unit IS NULL OR billing_unit = ''").
			Update("billing_unit", UserSubscriptionBillingUnitQuota).Error; err != nil {
			return err
		}
	}
	if err := DB.Model(&User{}).Where("base_multiplier IS NULL OR base_multiplier <= 0").Update("base_multiplier", 1).Error; err != nil {
		return err
	}
	if shouldApplyLegacyGroupOptionMigrations() {
		if err := runStepTx("BackfillGroupRatioEnsureLegacyDefaultModelGroup", BackfillGroupRatioEnsureLegacyDefaultModelGroup); err != nil {
			return err
		}
		if err := runStepTx("BackfillUserUsableGroupsEnsureLegacyDefaultModelGroup", BackfillUserUsableGroupsEnsureLegacyDefaultModelGroup); err != nil {
			return err
		}
	} else {
		common.SysLog("skip startup legacy group option migrations (disabled by default)")
	}
	if err := runStepTx("BackfillGroupsFromLegacyGroupsTable", BackfillGroupsFromLegacyGroupsTable); err != nil {
		return err
	}
	if err := runStepTx("BackfillGroupsFromLegacyOptions", BackfillGroupsFromLegacyOptions); err != nil {
		return err
	}
	if err := runStepTx("BackfillUserGroupsFromModelGroupsTx", BackfillUserGroupsFromModelGroupsTx); err != nil {
		return err
	}
	if err := runStepTx("EnsureGroupsCoverReferences", EnsureGroupsCoverReferences); err != nil {
		return err
	}
	if shouldApplyLegacyGroupOptionMigrations() {
		if err := runStepTx("BackfillGroupIDOptionsFromLegacyOptions", BackfillGroupIDOptionsFromLegacyOptions); err != nil {
			return err
		}
	} else {
		common.SysLog("skip group-id option rewrites for rollback safety")
	}
	if err := runStepTx("BackfillRedemptionPresetAllowedGroupsDefaults", BackfillRedemptionPresetAllowedGroupsDefaults); err != nil {
		return err
	}
	if err := runStepTx("BackfillGroupBindingsFromLegacyData", BackfillGroupBindingsFromLegacyData); err != nil {
		return err
	}
	if err := runStepTx("BackfillTokenAllowedGroupsSortOrder", BackfillTokenAllowedGroupsSortOrder); err != nil {
		return err
	}
	if err := runStepTx("BackfillPaygUserBalancesAllowedGroupIDs", BackfillPaygUserBalancesAllowedGroupIDs); err != nil {
		return err
	}
	if err := runStepTx("BackfillPayRequestUserBalancesAllowedGroupIDs", BackfillPayRequestUserBalancesAllowedGroupIDs); err != nil {
		return err
	}
	if err := runStepTx("BackfillUserSubscriptionSourceRefs", BackfillUserSubscriptionSourceRefs); err != nil {
		return err
	}
		if err := runStepTx("BackfillRedemptionPresetRevisions", BackfillRedemptionPresetRevisions); err != nil {
			return err
		}
		if err := runStepTx("BackfillSubscriptionOrderPresetRevisions", BackfillSubscriptionOrderPresetRevisions); err != nil {
			return err
		}
		if err := runStepTx("BackfillUserPresetRevisionBindings", BackfillUserPresetRevisionBindings); err != nil {
			return err
		}
		if err := runStepTx("BackfillPayProductRevisions", BackfillPayProductRevisions); err != nil {
			return err
		}
		if shouldReconcileLegacyTokenGroup() {
			if err := runStepTx("BackfillTokenGroupEnsureInAllowedGroups", BackfillTokenGroupEnsureInAllowedGroups); err != nil {
				return err
			}
		} else {
			common.SysLog("skip legacy token.group reconciliation for rollback safety")
		}
		if err := runStepTx("BackfillInvitationIsFirstPurchasePaidEvents", BackfillInvitationIsFirstPurchasePaidEvents); err != nil {
			return err
		}
		if err := runStepTx("BackfillPaygUserBalancesFromLegacyUsers", BackfillPaygUserBalancesFromLegacyUsers); err != nil {
			return err
		}
		if err := runStepTx("BackfillPayRequestUserBalancesFromLegacyUsers", BackfillPayRequestUserBalancesFromLegacyUsers); err != nil {
			return err
		}
		if err := runStepTx("BackfillPayTokenUserBalancesFromLegacyUsers", BackfillPayTokenUserBalancesFromLegacyUsers); err != nil {
			return err
		}
		if shouldSyncLegacyUserQuotaSnapshots() {
			if err := runStepTx("BackfillUsersPaygSnapshotFromBalances", BackfillUsersPaygSnapshotFromBalances); err != nil {
				return err
			}
			if err := runStepTx("BackfillUsersPayRequestSnapshotFromBalances", BackfillUsersPayRequestSnapshotFromBalances); err != nil {
				return err
			}
			if err := runStepTx("BackfillUsersPayTokenSnapshotFromBalances", BackfillUsersPayTokenSnapshotFromBalances); err != nil {
				return err
			}
		} else {
			common.SysLog("skip legacy user quota snapshot sync for rollback safety")
		}
		if err := runStepTx("BackfillUserPricingProfiles", BackfillUserPricingProfiles); err != nil {
			return err
		}
		if shouldMigrateDiscreteQuotaStorage() {
			if err := runStepTx("MigrateDiscreteQuotaStorageToScaledBigint", MigrateDiscreteQuotaStorageToScaledBigint); err != nil {
				return err
			}
		} else {
			common.SysLog("skip discrete quota storage migration for rollback safety")
		}
	if err := runStep("RefreshPricingRuleCache", func() error { return RefreshPricingRuleCache() }); err != nil {
		return err
	}
	return nil
}

func migrateDBFast() error {

	var wg sync.WaitGroup

	migrations := []struct {
		model interface{}
		name  string
	}{
		{&UserGroup{}, "UserGroup"},
		{&Group{}, "Group"},
		{&ChannelGroup{}, "ChannelGroup"},
		{&ChannelBackupGroup{}, "ChannelBackupGroup"},
		{&TokenAllowedGroup{}, "TokenAllowedGroup"},
		{&SubscriptionProductGroup{}, "SubscriptionProductGroup"},
		{&PaygProductRevision{}, "PaygProductRevision"},
		{&PaygProductRevisionGroup{}, "PaygProductRevisionGroup"},
		{&PayRequestProductRevision{}, "PayRequestProductRevision"},
		{&PayRequestProductRevisionGroup{}, "PayRequestProductRevisionGroup"},
		{&PayTokenProductRevision{}, "PayTokenProductRevision"},
		{&PayTokenProductRevisionGroup{}, "PayTokenProductRevisionGroup"},
		{&RedemptionPresetRevision{}, "RedemptionPresetRevision"},
		{&RedemptionPresetRevisionGroup{}, "RedemptionPresetRevisionGroup"},
		{&RedemptionPresetRevisionGroupDailyLimit{}, "RedemptionPresetRevisionGroupDailyLimit"},
		{&SubscriptionProductGroupDailyLimit{}, "SubscriptionProductGroupDailyLimit"},
		{&RedemptionGroupDailyLimit{}, "RedemptionGroupDailyLimit"},
		{&UserSubscriptionGroupDailyLimit{}, "UserSubscriptionGroupDailyLimit"},
		{&UserSubscriptionGroupDailyUsage{}, "UserSubscriptionGroupDailyUsage"},
		{&PaygProduct{}, "PaygProduct"},
		{&PaygProductGroup{}, "PaygProductGroup"},
		{&PayRequestProduct{}, "PayRequestProduct"},
		{&PayRequestProductGroup{}, "PayRequestProductGroup"},
		{&PayTokenProduct{}, "PayTokenProduct"},
		{&PayTokenProductGroup{}, "PayTokenProductGroup"},
		{&UserSubscriptionGroup{}, "UserSubscriptionGroup"},
		{&UserRequestSubscriptionGroup{}, "UserRequestSubscriptionGroup"},
		{&Channel{}, "Channel"},
		{&ChannelRequestDailyStat{}, "ChannelRequestDailyStat"},
		{&UserRequestDailyStat{}, "UserRequestDailyStat"},
		{&ChannelUserBinding{}, "ChannelUserBinding"},
		{&Token{}, "Token"},
		{&User{}, "User"},
		{&BalanceRecord{}, "BalanceRecord"},
		{&Option{}, "Option"},
		{&ClawBoxPortableRelease{}, "ClawBoxPortableRelease"},
		{&Redemption{}, "Redemption"},
		{&RedemptionPreset{}, "RedemptionPreset"},
		{&Ability{}, "Ability"},
		{&Log{}, "Log"},
		{&RequestTraceSession{}, "RequestTraceSession"},
		{&RequestTraceNode{}, "RequestTraceNode"},
		{&Midjourney{}, "Midjourney"},
		{&TopUp{}, "TopUp"},
		{&QuotaData{}, "QuotaData"},
		{&Task{}, "Task"},
		{&Model{}, "Model"},
		{&Vendor{}, "Vendor"},
		{&PrefillGroup{}, "PrefillGroup"},
		{&Setup{}, "Setup"},
		{&TwoFA{}, "TwoFA"},
		{&TwoFABackupCode{}, "TwoFABackupCode"},
		{&UserSubscription{}, "UserSubscription"},
		{&UserRequestSubscription{}, "UserRequestSubscription"},
		{&UserSubscriptionPresetRevisionBinding{}, "UserSubscriptionPresetRevisionBinding"},
		{&UserRequestSubscriptionPresetRevisionBinding{}, "UserRequestSubscriptionPresetRevisionBinding"},
		{&SubscriptionPlan{}, "SubscriptionPlan"},
		{&SubscriptionOrder{}, "SubscriptionOrder"},
		{&PaygOrder{}, "PaygOrder"},
		{&PayRequestOrder{}, "PayRequestOrder"},
		{&PayTokenOrder{}, "PayTokenOrder"},
		{&PaygUserBalance{}, "PaygUserBalance"},
		{&PayRequestUserBalance{}, "PayRequestUserBalance"},
		{&PayTokenUserBalance{}, "PayTokenUserBalance"},
		{&UserMembership{}, "UserMembership"},
		{&ClawBoxDeviceSession{}, "ClawBoxDeviceSession"},
		{&ClawBoxPortableBinding{}, "ClawBoxPortableBinding"},
	}
	// 动态计算migration数量，确保errChan缓冲区足够大
	errChan := make(chan error, len(migrations))

	for _, m := range migrations {
		wg.Add(1)
		go func(model interface{}, name string) {
			defer wg.Done()
			if err := DB.AutoMigrate(model); err != nil {
				errChan <- fmt.Errorf("failed to migrate %s: %v", name, err)
			}
		}(m.model, m.name)
	}

	// Wait for all migrations to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	common.SysLog("database migrated")
	return nil
}

func migrateLOGDB() error {
	var err error
	if err = LOG_DB.AutoMigrate(&Log{}, &ServiceStatusBucketStat{}, &ServiceStatusRequestState{}, &ServiceStatusBucketStatsMeta{}); err != nil {
		return err
	}
	return nil
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}

func CloseDB() error {
	if LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return err
		}
	}
	return closeDB(DB)
}

// checkMySQLChineseSupport ensures the MySQL connection and current schema
// default charset/collation can store Chinese characters. It allows common
// Chinese-capable charsets (utf8mb4, utf8, gbk, big5, gb18030) and panics otherwise.
func checkMySQLChineseSupport(db *gorm.DB) error {
	// 仅检测：当前库默认字符集/排序规则 + 各表的排序规则（隐含字符集）

	// Read current schema defaults
	var schemaCharset, schemaCollation string
	err := db.Raw("SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = DATABASE()").Row().Scan(&schemaCharset, &schemaCollation)
	if err != nil {
		return fmt.Errorf("读取当前库默认字符集/排序规则失败 / Failed to read schema default charset/collation: %v", err)
	}

	toLower := func(s string) string { return strings.ToLower(s) }
	// Allowed charsets that can store Chinese text
	allowedCharsets := map[string]string{
		"utf8mb4": "utf8mb4_",
		"utf8":    "utf8_",
		"gbk":     "gbk_",
		"big5":    "big5_",
		"gb18030": "gb18030_",
	}
	isChineseCapable := func(cs, cl string) bool {
		csLower := toLower(cs)
		clLower := toLower(cl)
		if prefix, ok := allowedCharsets[csLower]; ok {
			if clLower == "" {
				return true
			}
			return strings.HasPrefix(clLower, prefix)
		}
		// 如果仅提供了排序规则，尝试按排序规则前缀判断
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(clLower, prefix) {
				return true
			}
		}
		return false
	}

	// 1) 当前库默认值必须支持中文
	if !isChineseCapable(schemaCharset, schemaCollation) {
		return fmt.Errorf("当前库默认字符集/排序规则不支持中文：schema(%s/%s)。请将库设置为 utf8mb4/utf8/gbk/big5/gb18030 / Schema default charset/collation is not Chinese-capable: schema(%s/%s). Please set to utf8mb4/utf8/gbk/big5/gb18030",
			schemaCharset, schemaCollation, schemaCharset, schemaCollation)
	}

	// 2) 所有物理表的排序规则（隐含字符集）必须支持中文
	type tableInfo struct {
		Name      string
		Collation *string
	}
	var tables []tableInfo
	if err := db.Raw("SELECT TABLE_NAME, TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'").Scan(&tables).Error; err != nil {
		return fmt.Errorf("读取表排序规则失败 / Failed to read table collations: %v", err)
	}

	var badTables []string
	for _, t := range tables {
		// NULL 或空表示继承库默认设置，已在上面校验库默认，视为通过
		if t.Collation == nil || *t.Collation == "" {
			continue
		}
		cl := *t.Collation
		// 仅凭排序规则判断是否中文可用
		ok := false
		lower := strings.ToLower(cl)
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(lower, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			badTables = append(badTables, fmt.Sprintf("%s(%s)", t.Name, cl))
		}
	}

	if len(badTables) > 0 {
		// 限制输出数量以避免日志过长
		maxShow := 20
		shown := badTables
		if len(shown) > maxShow {
			shown = shown[:maxShow]
		}
		return fmt.Errorf(
			"存在不支持中文的表，请修复其排序规则/字符集。示例（最多展示 %d 项）：%v / Found tables not Chinese-capable. Please fix their collation/charset. Examples (showing up to %d): %v",
			maxShow, shown, maxShow, shown,
		)
	}
	return nil
}

var (
	lastPingTime time.Time
	pingMutex    sync.Mutex
)

func PingDB() error {
	pingMutex.Lock()
	defer pingMutex.Unlock()

	if time.Since(lastPingTime) < time.Second*10 {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Error getting sql.DB from GORM: %v", err)
		return err
	}

	err = sqlDB.Ping()
	if err != nil {
		log.Printf("Error pinging DB: %v", err)
		return err
	}

	lastPingTime = time.Now()
	common.SysLog("Database pinged successfully")
	return nil
}
