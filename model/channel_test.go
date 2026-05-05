package model

import "testing"

func TestParseChannelKeyListJSONStringArray(t *testing.T) {
	keys := ParseChannelKeyList(`["sk-one","sk-two"]`)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0] != "sk-one" || keys[1] != "sk-two" {
		t.Fatalf("unexpected keys: %#v", keys)
	}
}

func TestParseChannelKeyListJSONObjectArray(t *testing.T) {
	keys := ParseChannelKeyList(`[{"project":"a"},{"project":"b"}]`)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0] != `{"project":"a"}` || keys[1] != `{"project":"b"}` {
		t.Fatalf("unexpected keys: %#v", keys)
	}
}
