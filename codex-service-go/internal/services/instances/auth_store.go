package instances

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type AuthRecord struct {
	InstanceID           int64
	AuthJSON             string
	DupKey               string
	AccountID            string
	AccountType          string
	LastRefresh          string
	AccessTokenExpiresAt int64
	ImportedAt           time.Time
	UpdatedAt            time.Time
}

type AuthMeta struct {
	InstanceID           int64
	DupKey               string
	AccountID            string
	AccountType          string
	LastRefresh          string
	AccessTokenExpiresAt int64
	UpdatedAt            time.Time
}

func (r *Repository) UpsertAuth(ctx context.Context, instanceID int64, authJSON string, dupKey string, accountID string, accountType string, lastRefresh string, accessTokenExpiresAt *int64) error {
	if err := r.ensureRepaired(); err != nil {
		return err
	}
	var exists int
	if err := r.db.QueryRowContext(ctx, r.formatQuery("SELECT 1 FROM "+r.instancesTable+" WHERE id = ?"), instanceID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("instance not found")
		}
		return err
	}

	var expires any
	if accessTokenExpiresAt != nil && *accessTokenExpiresAt > 0 {
		expires = *accessTokenExpiresAt
	} else {
		expires = nil
	}
	now := time.Now()
	nowUnix := now.Unix()
	timeNodes := []int64{}
	{
		var importedAt sql.NullTime
		var existingNodes sql.NullString
		err := r.db.QueryRowContext(ctx, r.formatQuery("SELECT imported_at, time_nodes FROM "+r.instanceAuthTable+" WHERE instance_id = ?"), instanceID).
			Scan(&importedAt, &existingNodes)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			raw := strings.TrimSpace(existingNodes.String)
			if raw != "" {
				if e := json.Unmarshal([]byte(raw), &timeNodes); e != nil {
					return fmt.Errorf("parse time_nodes for instance %d: %w", instanceID, e)
				}
			}
			if len(timeNodes) == 0 && importedAt.Valid && !importedAt.Time.IsZero() {
				timeNodes = append(timeNodes, importedAt.Time.Unix())
			}
		}
		if len(timeNodes) == 0 || timeNodes[len(timeNodes)-1] != nowUnix {
			timeNodes = append(timeNodes, nowUnix)
		}
	}
	timeNodesJSON, err := json.Marshal(timeNodes)
	if err != nil {
		return err
	}
	switch r.dialect {
	case "mysql":
		_, err := r.db.ExecContext(ctx, r.formatQuery(`
	INSERT INTO `+r.instanceAuthTable+` (instance_id, auth_json, dup_key, account_id, account_type, last_refresh, access_token_expires_at, time_nodes, imported_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
	  auth_json = VALUES(auth_json),
	  dup_key = VALUES(dup_key),
	  account_id = VALUES(account_id),
	  account_type = VALUES(account_type),
	  last_refresh = VALUES(last_refresh),
	  access_token_expires_at = VALUES(access_token_expires_at),
	  time_nodes = VALUES(time_nodes),
	  imported_at = IFNULL(imported_at, VALUES(imported_at)),
	  updated_at = VALUES(updated_at)
	`), instanceID, authJSON, nullIfEmpty(dupKey), nullIfEmpty(accountID), nullIfEmpty(accountType), nullIfEmpty(lastRefresh), expires, string(timeNodesJSON), now, now)
		return err
	default:
		_, err := r.db.ExecContext(ctx, r.formatQuery(`
	INSERT INTO `+r.instanceAuthTable+` (instance_id, auth_json, dup_key, account_id, account_type, last_refresh, access_token_expires_at, time_nodes, imported_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(instance_id) DO UPDATE SET
	  auth_json = excluded.auth_json,
	  dup_key = excluded.dup_key,
	  account_id = excluded.account_id,
	  account_type = excluded.account_type,
	  last_refresh = excluded.last_refresh,
	  access_token_expires_at = excluded.access_token_expires_at,
	  time_nodes = excluded.time_nodes,
	  imported_at = COALESCE(`+r.instanceAuthTable+`.imported_at, excluded.imported_at),
	  updated_at = excluded.updated_at
	`), instanceID, authJSON, nullIfEmpty(dupKey), nullIfEmpty(accountID), nullIfEmpty(accountType), nullIfEmpty(lastRefresh), expires, string(timeNodesJSON), now, now)
		return err
	}
}

func (r *Repository) GetAuth(ctx context.Context, instanceID int64) (*AuthRecord, error) {
	if err := r.ensureRepaired(); err != nil {
		return nil, err
	}
	var rec AuthRecord
	var dupKey sql.NullString
	var accountID sql.NullString
	var accountType sql.NullString
	var lastRefresh sql.NullString
	var expires sql.NullInt64
	err := r.db.QueryRowContext(ctx, r.formatQuery(`
SELECT a.instance_id, a.auth_json, a.dup_key, a.account_id, a.account_type, a.last_refresh, a.access_token_expires_at, a.imported_at, a.updated_at
FROM `+r.instanceAuthTable+` a
JOIN `+r.instancesTable+` i ON i.id = a.instance_id
WHERE a.instance_id = ?
`), instanceID).Scan(&rec.InstanceID, &rec.AuthJSON, &dupKey, &accountID, &accountType, &lastRefresh, &expires, &rec.ImportedAt, &rec.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec.DupKey = dupKey.String
	rec.AccountID = accountID.String
	rec.AccountType = accountType.String
	rec.LastRefresh = lastRefresh.String
	if expires.Valid {
		rec.AccessTokenExpiresAt = expires.Int64
	}
	return &rec, nil
}

func (r *Repository) GetAuthMeta(ctx context.Context, instanceID int64) (*AuthMeta, error) {
	if err := r.ensureRepaired(); err != nil {
		return nil, err
	}
	var meta AuthMeta
	var dupKey sql.NullString
	var accountID sql.NullString
	var accountType sql.NullString
	var lastRefresh sql.NullString
	var expires sql.NullInt64
	err := r.db.QueryRowContext(ctx, r.formatQuery(`
SELECT a.instance_id, a.dup_key, a.account_id, a.account_type, a.last_refresh, a.access_token_expires_at, a.updated_at
FROM `+r.instanceAuthTable+` a
JOIN `+r.instancesTable+` i ON i.id = a.instance_id
WHERE a.instance_id = ?
`), instanceID).Scan(&meta.InstanceID, &dupKey, &accountID, &accountType, &lastRefresh, &expires, &meta.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	meta.DupKey = dupKey.String
	meta.AccountID = accountID.String
	meta.AccountType = accountType.String
	meta.LastRefresh = lastRefresh.String
	if expires.Valid {
		meta.AccessTokenExpiresAt = expires.Int64
	}
	return &meta, nil
}

func (r *Repository) ListAuthMeta(ctx context.Context) ([]AuthMeta, error) {
	if err := r.ensureRepaired(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT a.instance_id, a.dup_key, a.account_id, a.account_type, a.last_refresh, a.access_token_expires_at, a.updated_at
FROM `+r.instanceAuthTable+` a
JOIN `+r.instancesTable+` i ON i.id = a.instance_id
ORDER BY a.instance_id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuthMeta
	for rows.Next() {
		var meta AuthMeta
		var dupKey sql.NullString
		var accountID sql.NullString
		var accountType sql.NullString
		var lastRefresh sql.NullString
		var expires sql.NullInt64
		if err := rows.Scan(&meta.InstanceID, &dupKey, &accountID, &accountType, &lastRefresh, &expires, &meta.UpdatedAt); err != nil {
			return nil, err
		}
		meta.DupKey = dupKey.String
		meta.AccountID = accountID.String
		meta.AccountType = accountType.String
		meta.LastRefresh = lastRefresh.String
		if expires.Valid {
			meta.AccessTokenExpiresAt = expires.Int64
		}
		out = append(out, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) ListAuth(ctx context.Context) ([]AuthRecord, error) {
	if err := r.ensureRepaired(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT a.instance_id, a.auth_json, a.dup_key, a.account_id, a.account_type, a.last_refresh, a.access_token_expires_at, a.imported_at, a.updated_at
FROM `+r.instanceAuthTable+` a
JOIN `+r.instancesTable+` i ON i.id = a.instance_id
ORDER BY a.instance_id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuthRecord
	for rows.Next() {
		var rec AuthRecord
		var dupKey sql.NullString
		var accountID sql.NullString
		var accountType sql.NullString
		var lastRefresh sql.NullString
		var expires sql.NullInt64
		if err := rows.Scan(&rec.InstanceID, &rec.AuthJSON, &dupKey, &accountID, &accountType, &lastRefresh, &expires, &rec.ImportedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		rec.DupKey = dupKey.String
		rec.AccountID = accountID.String
		rec.AccountType = accountType.String
		rec.LastRefresh = lastRefresh.String
		if expires.Valid {
			rec.AccessTokenExpiresAt = expires.Int64
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) DeleteAuth(ctx context.Context, instanceID int64) error {
	_, err := r.db.ExecContext(ctx, r.formatQuery("DELETE FROM "+r.instanceAuthTable+" WHERE instance_id = ?"), instanceID)
	return err
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}
