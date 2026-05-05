package instances

import "testing"

func TestWithRevisionBatchDefersNotificationUntilExit(t *testing.T) {
	svc := NewService(nil, Options{})
	before := svc.CurrentRevision()
	waitCh := svc.revisionChannel()

	if err := svc.WithRevisionBatch(func() error {
		if got := svc.TouchRevision(); got != before {
			t.Fatalf("expected batch touch to keep revision %d, got %d", before, got)
		}
		if cur := svc.CurrentRevision(); cur != before {
			t.Fatalf("expected current revision to stay %d during batch, got %d", before, cur)
		}
		select {
		case <-waitCh:
			t.Fatal("revision channel closed before batch exit")
		default:
		}
		svc.TouchRevision()
		return nil
	}); err != nil {
		t.Fatalf("WithRevisionBatch returned error: %v", err)
	}

	after := svc.CurrentRevision()
	if after <= before {
		t.Fatalf("expected revision to advance after batch, before=%d after=%d", before, after)
	}
	select {
	case <-waitCh:
	default:
		t.Fatal("expected revision channel to close after batch exit")
	}
}
