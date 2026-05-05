package embed

import instsvc "codex-service-go/internal/services/instances"

// Re-export internal instance types so the embedding host (Transfer API) can use the
// services without importing codex-service-go/internal (which is disallowed by Go).
type (
	Instance          = instsvc.Instance
	InstanceWithPaths = instsvc.InstanceWithPaths
	CreateParams      = instsvc.CreateParams
	UpdateParams      = instsvc.UpdateParams
	DebugConfig       = instsvc.DebugConfig
	AuthMeta          = instsvc.AuthMeta
	ValidationError   = instsvc.ValidationError
)
