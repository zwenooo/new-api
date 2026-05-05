package proxy

import (
	"testing"

	instsvc "codex-service-go/internal/services/instances"
)

func TestWebsocketDialerForInstance_EnablesCompression(t *testing.T) {
	dialer, err := websocketDialerForInstance(instsvc.InstanceWithPaths{})
	if err != nil {
		t.Fatalf("websocketDialerForInstance error: %v", err)
	}
	if dialer == nil {
		t.Fatal("expected websocket dialer")
	}
	if !dialer.EnableCompression {
		t.Fatal("expected websocket dialer to enable compression negotiation")
	}
}
