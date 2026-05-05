package instances

import "context"

type CodexUsageSnapshotUpdate struct {
	UpdatedAt     int64
	Used5hPercent *float64
	Reset5hAt     *int64
	Used7dPercent *float64
	Reset7dAt     *int64
}

func (r *Repository) UpdateCodexUsageSnapshot(ctx context.Context, instanceID int64, snapshot CodexUsageSnapshotUpdate) error {
	if r == nil || r.db == nil || instanceID <= 0 {
		return nil
	}
	_, err := r.db.ExecContext(
		ctx,
		r.formatQuery("UPDATE "+r.instancesTable+" SET codex_usage_updated_at = ?, codex_5h_used_percent = ?, codex_5h_reset_at = ?, codex_7d_used_percent = ?, codex_7d_reset_at = ? WHERE id = ?"),
		nullableInt64(snapshot.UpdatedAt),
		snapshot.Used5hPercent,
		snapshot.Reset5hAt,
		snapshot.Used7dPercent,
		snapshot.Reset7dAt,
		instanceID,
	)
	return err
}

func (s *Service) SaveCodexUsageSnapshot(ctx context.Context, id int64, updatedAt int64, used5hPercent *float64, reset5hAt *int64, used7dPercent *float64, reset7dAt *int64) error {
	if s == nil || s.repo == nil || id <= 0 {
		return nil
	}
	return s.repo.UpdateCodexUsageSnapshot(ctx, id, CodexUsageSnapshotUpdate{
		UpdatedAt:     updatedAt,
		Used5hPercent: used5hPercent,
		Reset5hAt:     reset5hAt,
		Used7dPercent: used7dPercent,
		Reset7dAt:     reset7dAt,
	})
}

func nullableInt64(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}
