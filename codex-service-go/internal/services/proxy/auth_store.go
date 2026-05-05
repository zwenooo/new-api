package proxy

import (
	"context"

	instsvc "codex-service-go/internal/services/instances"
)

type AuthStore interface {
	GetAuth(ctx context.Context, instanceID int64) (*instsvc.AuthRecord, error)
	SaveAuth(ctx context.Context, instanceID int64, authJSON string) error
}

