package instances

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Instance struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	InternalToken   string    `json:"internal_token"`
	UpstreamBaseURL string    `json:"upstream_base_url"`
	UpstreamAPIKey  string    `json:"upstream_api_key"`
	Proxy           string    `json:"proxy"`
	AuthMode        string    `json:"auth_mode"`
	Enabled         bool      `json:"enabled"`
	DebugEnabled    bool      `json:"debug_enabled"`
	DebugDetailEnabled         bool `json:"debug_detail_enabled"`
	DebugLogReqHeaders         bool `json:"debug_log_req_headers"`
	DebugLogReqBody            bool `json:"debug_log_req_body"`
	DebugLogReqBodyMode        int  `json:"debug_log_req_body_mode"`
	DebugLogRedactHeaders      bool `json:"debug_log_redact_headers"`
	DebugSSECompressOutputTextDelta bool `json:"debug_sse_compress_output_text_delta"`
	DebugSSEKeepalive          bool `json:"debug_sse_keepalive"`
	DebugSSEMaskInstructions   bool `json:"debug_sse_mask_instructions"`
	DebugSSEMaskText           bool `json:"debug_sse_mask_text"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RepositoryOptions struct {
	TablePrefix string
	Dialect     string
}

type Repository struct {
	db               *sql.DB
	instancesTable   string
	instanceAuthTable string
	dialect          string
	repairOnce       sync.Once
	repairErr        error
}

func (r *Repository) ensureRepaired() error {
	if r == nil || r.db == nil {
		return nil
	}
	r.repairOnce.Do(func() {
		r.repairErr = r.RepairNullTimestamps(context.Background())
	})
	return r.repairErr
}

func (r *Repository) formatQuery(query string) string {
	if r == nil {
		return query
	}
	switch strings.ToLower(strings.TrimSpace(r.dialect)) {
	case "postgres", "postgresql", "pg":
		n := 1
		var b strings.Builder
		b.Grow(len(query) + 8)
		for i := 0; i < len(query); i++ {
			if query[i] == '?' {
				b.WriteByte('$')
				b.WriteString(strconv.Itoa(n))
				n++
				continue
			}
			b.WriteByte(query[i])
		}
		return b.String()
	default:
		return query
	}
}

func NewRepository(db *sql.DB) *Repository {
	return NewRepositoryWithOptions(db, RepositoryOptions{})
}

func NewRepositoryWithOptions(db *sql.DB, opts RepositoryOptions) *Repository {
	prefix := strings.TrimSpace(opts.TablePrefix)
	dialect := strings.ToLower(strings.TrimSpace(opts.Dialect))
	if dialect == "" {
		dialect = "sqlite"
	}
	return &Repository{
		db:               db,
		instancesTable:   prefix + "instances",
		instanceAuthTable: prefix + "instance_auth",
		dialect:          dialect,
	}
}

func (r *Repository) RepairNullTimestamps(ctx context.Context) error {
	if r == nil || r.db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if _, err := r.db.ExecContext(ctx, "UPDATE "+r.instancesTable+" SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, "UPDATE "+r.instancesTable+" SET updated_at = created_at WHERE updated_at IS NULL"); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, "UPDATE "+r.instanceAuthTable+" SET imported_at = CURRENT_TIMESTAMP WHERE imported_at IS NULL"); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, "UPDATE "+r.instanceAuthTable+" SET updated_at = imported_at WHERE updated_at IS NULL"); err != nil {
		return err
	}
	return nil
}

func (r *Repository) List(ctx context.Context) ([]Instance, error) {
	if err := r.ensureRepaired(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT id, name, internal_token, upstream_base_url, upstream_api_key, proxy, auth_mode, enabled, debug_enabled, debug_detail_enabled, debug_log_req_headers, debug_log_req_body, debug_log_req_body_mode, debug_log_redact_headers, debug_sse_compress_output_text_delta, debug_sse_keepalive, debug_sse_mask_instructions, debug_sse_mask_text, created_at, updated_at FROM "+r.instancesTable+" ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Instance
	for rows.Next() {
		var inst Instance
		var enabledInt int
		var debugInt int
		var debugDetailInt int
		var debugLogReqHeadersInt int
		var debugLogReqBodyInt int
		var debugLogReqBodyModeInt int
		var debugLogRedactHeadersInt int
		var debugSSECompressDeltaInt int
		var debugSSEKeepaliveInt int
		var debugSSEMaskInstructionsInt int
		var debugSSEMaskTextInt int
		if err := rows.Scan(
			&inst.ID,
			&inst.Name,
			&inst.InternalToken,
			&inst.UpstreamBaseURL,
			&inst.UpstreamAPIKey,
			&inst.Proxy,
			&inst.AuthMode,
			&enabledInt,
			&debugInt,
			&debugDetailInt,
			&debugLogReqHeadersInt,
			&debugLogReqBodyInt,
			&debugLogReqBodyModeInt,
			&debugLogRedactHeadersInt,
			&debugSSECompressDeltaInt,
			&debugSSEKeepaliveInt,
			&debugSSEMaskInstructionsInt,
			&debugSSEMaskTextInt,
			&inst.CreatedAt,
			&inst.UpdatedAt,
		); err != nil {
			return nil, err
		}
		inst.Enabled = enabledInt != 0
		inst.DebugEnabled = debugInt != 0
		inst.DebugDetailEnabled = debugDetailInt != 0
		inst.DebugLogReqHeaders = debugLogReqHeadersInt != 0
		inst.DebugLogReqBody = debugLogReqBodyInt != 0
		inst.DebugLogReqBodyMode = debugLogReqBodyModeInt
		inst.DebugLogRedactHeaders = debugLogRedactHeadersInt != 0
		inst.DebugSSECompressOutputTextDelta = debugSSECompressDeltaInt != 0
		inst.DebugSSEKeepalive = debugSSEKeepaliveInt != 0
		inst.DebugSSEMaskInstructions = debugSSEMaskInstructionsInt != 0
		inst.DebugSSEMaskText = debugSSEMaskTextInt != 0
		out = append(out, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*Instance, error) {
	if err := r.ensureRepaired(); err != nil {
		return nil, err
	}
	var inst Instance
	var enabledInt int
	var debugInt int
	var debugDetailInt int
	var debugLogReqHeadersInt int
	var debugLogReqBodyInt int
	var debugLogReqBodyModeInt int
	var debugLogRedactHeadersInt int
	var debugSSECompressDeltaInt int
	var debugSSEKeepaliveInt int
	var debugSSEMaskInstructionsInt int
	var debugSSEMaskTextInt int
	err := r.db.QueryRowContext(ctx, r.formatQuery("SELECT id, name, internal_token, upstream_base_url, upstream_api_key, proxy, auth_mode, enabled, debug_enabled, debug_detail_enabled, debug_log_req_headers, debug_log_req_body, debug_log_req_body_mode, debug_log_redact_headers, debug_sse_compress_output_text_delta, debug_sse_keepalive, debug_sse_mask_instructions, debug_sse_mask_text, created_at, updated_at FROM "+r.instancesTable+" WHERE id = ?"), id).
		Scan(
			&inst.ID,
			&inst.Name,
			&inst.InternalToken,
			&inst.UpstreamBaseURL,
			&inst.UpstreamAPIKey,
			&inst.Proxy,
			&inst.AuthMode,
			&enabledInt,
			&debugInt,
			&debugDetailInt,
			&debugLogReqHeadersInt,
			&debugLogReqBodyInt,
			&debugLogReqBodyModeInt,
			&debugLogRedactHeadersInt,
			&debugSSECompressDeltaInt,
			&debugSSEKeepaliveInt,
			&debugSSEMaskInstructionsInt,
			&debugSSEMaskTextInt,
			&inst.CreatedAt,
			&inst.UpdatedAt,
		)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	inst.Enabled = enabledInt != 0
	inst.DebugEnabled = debugInt != 0
	inst.DebugDetailEnabled = debugDetailInt != 0
	inst.DebugLogReqHeaders = debugLogReqHeadersInt != 0
	inst.DebugLogReqBody = debugLogReqBodyInt != 0
	inst.DebugLogReqBodyMode = debugLogReqBodyModeInt
	inst.DebugLogRedactHeaders = debugLogRedactHeadersInt != 0
	inst.DebugSSECompressOutputTextDelta = debugSSECompressDeltaInt != 0
	inst.DebugSSEKeepalive = debugSSEKeepaliveInt != 0
	inst.DebugSSEMaskInstructions = debugSSEMaskInstructionsInt != 0
	inst.DebugSSEMaskText = debugSSEMaskTextInt != 0
	return &inst, nil
}

type CreateParams struct {
	Name            string
	InternalToken   string
	UpstreamBaseURL string
	UpstreamAPIKey  string
	Proxy           string
	AuthMode        string
	Enabled         bool
}

func (r *Repository) Create(ctx context.Context, arg CreateParams) (*Instance, error) {
	if arg.AuthMode == "" {
		arg.AuthMode = "chatgpt"
	}
	en := 0
	if arg.Enabled {
		en = 1
	}
	now := time.Now()
	res, err := r.db.ExecContext(ctx, r.formatQuery("INSERT INTO "+r.instancesTable+" (name, internal_token, upstream_base_url, upstream_api_key, proxy, auth_mode, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"),
		arg.Name, arg.InternalToken, arg.UpstreamBaseURL, arg.UpstreamAPIKey, arg.Proxy, arg.AuthMode, en, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

type UpdateParams struct {
	ID              int64
	Name            string
	InternalToken   string
	UpstreamBaseURL string
	UpstreamAPIKey  string
	Proxy           *string
	AuthMode        string
	Enabled         *bool
}

func (r *Repository) Update(ctx context.Context, arg UpdateParams) (*Instance, error) {
	if arg.Proxy == nil {
		return nil, errors.New("proxy is required")
	}
	if arg.AuthMode == "" {
		arg.AuthMode = "chatgpt"
	}
	now := time.Now()
	if arg.Enabled != nil {
		en := 0
		if *arg.Enabled {
			en = 1
		}
		_, err := r.db.ExecContext(ctx, r.formatQuery("UPDATE "+r.instancesTable+" SET name = ?, internal_token = ?, upstream_base_url = ?, upstream_api_key = ?, proxy = ?, auth_mode = ?, enabled = ?, updated_at = ? WHERE id = ?"),
			arg.Name, arg.InternalToken, arg.UpstreamBaseURL, arg.UpstreamAPIKey, *arg.Proxy, arg.AuthMode, en, now, arg.ID)
		if err != nil {
			return nil, err
		}
	} else {
		_, err := r.db.ExecContext(ctx, r.formatQuery("UPDATE "+r.instancesTable+" SET name = ?, internal_token = ?, upstream_base_url = ?, upstream_api_key = ?, proxy = ?, auth_mode = ?, updated_at = ? WHERE id = ?"),
			arg.Name, arg.InternalToken, arg.UpstreamBaseURL, arg.UpstreamAPIKey, *arg.Proxy, arg.AuthMode, now, arg.ID)
		if err != nil {
			return nil, err
		}
	}
	return r.GetByID(ctx, arg.ID)
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.formatQuery("DELETE FROM "+r.instanceAuthTable+" WHERE instance_id = ?"), id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, r.formatQuery("DELETE FROM "+r.instancesTable+" WHERE id = ?"), id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return nil
}

func (r *Repository) SetEnabled(ctx context.Context, id int64, enable bool) error {
	en := 0
	if enable {
		en = 1
	}
	now := time.Now()
	_, err := r.db.ExecContext(ctx, r.formatQuery("UPDATE "+r.instancesTable+" SET enabled = ?, updated_at = ? WHERE id = ?"), en, now, id)
	return err
}

func (r *Repository) SetEnabledBatch(ctx context.Context, ids []int64, enable bool) error {
	if len(ids) == 0 {
		return nil
	}
	en := 0
	if enable {
		en = 1
	}
	now := time.Now()
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, en, now)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := "UPDATE " + r.instancesTable + " SET enabled = ?, updated_at = ? WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	_, err := r.db.ExecContext(ctx, r.formatQuery(query), args...)
	return err
}

func (r *Repository) SetDebugEnabled(ctx context.Context, id int64, enable bool) error {
	if !enable {
		now := time.Now()
		_, err := r.db.ExecContext(ctx, r.formatQuery("UPDATE "+r.instancesTable+" SET debug_enabled = 0, updated_at = ? WHERE id = ?"), now, id)
		return err
	}
	return r.SetDebugConfig(ctx, id, DebugConfig{
		Enabled:                  true,
		DetailEnabled:            false,
		LogReqHeaders:            true,
		LogReqBody:               true,
		LogReqBodyMode:           0,
		LogRedactHeaders:         false,
		SSECompressOutputTextDelta: true,
		SSEKeepalive:             true,
		SSEMaskInstructions:      true,
		SSEMaskText:              true,
	})
}

type DebugConfig struct {
	Enabled                  bool
	DetailEnabled            bool
	LogReqHeaders            bool
	LogReqBody               bool
	LogReqBodyMode           int
	LogRedactHeaders         bool
	SSECompressOutputTextDelta bool
	SSEKeepalive             bool
	SSEMaskInstructions      bool
	SSEMaskText              bool
}

func (r *Repository) SetDebugConfig(ctx context.Context, id int64, cfg DebugConfig) error {
	if !cfg.Enabled {
		now := time.Now()
		_, err := r.db.ExecContext(ctx, r.formatQuery("UPDATE "+r.instancesTable+" SET debug_enabled = 0, updated_at = ? WHERE id = ?"), now, id)
		return err
	}
	de := 0
	if cfg.DetailEnabled {
		de = 1
	}
	rh := 0
	if cfg.LogReqHeaders {
		rh = 1
	}
	rb := 0
	if cfg.LogReqBody {
		rb = 1
	}
	redactHeaders := 0
	if cfg.LogRedactHeaders {
		redactHeaders = 1
	}
	sseCompressDelta := 0
	if cfg.SSECompressOutputTextDelta {
		sseCompressDelta = 1
	}
	sseKeepalive := 0
	if cfg.SSEKeepalive {
		sseKeepalive = 1
	}
	sseMaskInstructions := 0
	if cfg.SSEMaskInstructions {
		sseMaskInstructions = 1
	}
	sseMaskText := 0
	if cfg.SSEMaskText {
		sseMaskText = 1
	}

	now := time.Now()
	_, err := r.db.ExecContext(
		ctx,
		r.formatQuery("UPDATE "+r.instancesTable+" SET debug_enabled = 1, debug_detail_enabled = ?, debug_log_req_headers = ?, debug_log_req_body = ?, debug_log_req_body_mode = ?, debug_log_redact_headers = ?, debug_sse_compress_output_text_delta = ?, debug_sse_keepalive = ?, debug_sse_mask_instructions = ?, debug_sse_mask_text = ?, updated_at = ? WHERE id = ?"),
		de,
		rh,
		rb,
		cfg.LogReqBodyMode,
		redactHeaders,
		sseCompressDelta,
		sseKeepalive,
		sseMaskInstructions,
		sseMaskText,
		now,
		id,
	)
	return err
}

func (r *Repository) SetAllInternalTokens(ctx context.Context, token string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, r.formatQuery("UPDATE "+r.instancesTable+" SET internal_token = ?, updated_at = ?"), token, now)
	return err
}
