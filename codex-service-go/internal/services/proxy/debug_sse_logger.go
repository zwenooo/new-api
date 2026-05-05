package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	instsvc "codex-service-go/internal/services/instances"
)

type sseLogOptions struct {
	compressOutputTextDelta bool
	keepalive               bool
	maskInstructions        bool
	maskText                bool
}

func (o sseLogOptions) needsProcessing() bool {
	if o.compressOutputTextDelta {
		return true
	}
	if !o.keepalive {
		return true
	}
	if o.maskInstructions || o.maskText {
		return true
	}
	return false
}

func wrapSSEBodyForLogging(rc io.ReadCloser, w io.WriteCloser, inst instsvc.InstanceWithPaths) io.ReadCloser {
	opts := sseLogOptions{
		compressOutputTextDelta: inst.DebugSSECompressOutputTextDelta,
		keepalive:               inst.DebugSSEKeepalive,
		maskInstructions:        inst.DebugSSEMaskInstructions,
		maskText:                inst.DebugSSEMaskText,
	}
	if !opts.needsProcessing() {
		return &teeReadCloser{rc: rc, w: w}
	}
	return newSSELogTeeReadCloser(rc, w, opts)
}

type suppressedDeltaStats struct {
	events int
	lines  int
	bytes  int64
	seqMin int64
	seqMax int64
	seqOK  bool
}

type sseLogTeeReadCloser struct {
	rc   io.ReadCloser
	w    io.WriteCloser
	opts sseLogOptions

	pending []byte

	curEvent         string
	suppressKeepalive bool
	suppressDelta     bool

	suppressedDelta suppressedDeltaStats
}

func newSSELogTeeReadCloser(rc io.ReadCloser, w io.WriteCloser, opts sseLogOptions) io.ReadCloser {
	return &sseLogTeeReadCloser{
		rc:   rc,
		w:    w,
		opts: opts,
	}
}

func (t *sseLogTeeReadCloser) Read(p []byte) (int, error) {
	n, err := t.rc.Read(p)
	if n > 0 {
		t.consume(p[:n])
	}
	return n, err
}

func (t *sseLogTeeReadCloser) Close() error {
	if len(t.pending) > 0 {
		// Flush remaining bytes as a final line (without forcing a trailing newline).
		t.processLine(t.pending)
		t.pending = nil
	}
	t.flushSuppressedDelta()

	err1 := t.rc.Close()
	err2 := t.w.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func (t *sseLogTeeReadCloser) consume(chunk []byte) {
	t.pending = append(t.pending, chunk...)
	for {
		idx := bytes.IndexByte(t.pending, '\n')
		if idx < 0 {
			return
		}
		line := t.pending[:idx+1]
		t.pending = t.pending[idx+1:]
		t.processLine(line)
	}
}

func (t *sseLogTeeReadCloser) processLine(line []byte) {
	trimmed := bytes.TrimRight(line, "\r\n")
	if len(trimmed) == 0 {
		// Blank line terminates current SSE event.
		if t.suppressDelta {
			t.addSuppressedDeltaLine(line)
			t.suppressDelta = false
			t.curEvent = ""
			t.suppressedDelta.events++
			return
		}
		if t.suppressKeepalive {
			t.suppressKeepalive = false
			t.curEvent = ""
			return
		}

		t.flushSuppressedDelta()
		t.curEvent = ""
		t.writeLine(line)
		return
	}

	if bytes.HasPrefix(trimmed, []byte("event:")) {
		eventName := bytes.TrimSpace(trimmed[len("event:"):])

		if t.suppressDelta {
			// Next event begins before delimiter; end previous delta.
			t.suppressDelta = false
			t.curEvent = ""
			t.suppressedDelta.events++
		}
		if t.suppressKeepalive {
			t.suppressKeepalive = false
			t.curEvent = ""
		}

		if bytes.Equal(eventName, []byte("response.output_text.delta")) && t.opts.compressOutputTextDelta {
			t.curEvent = "response.output_text.delta"
			t.suppressDelta = true
			t.addSuppressedDeltaLine(line)
			return
		}

		if bytes.Equal(eventName, []byte("keepalive")) && !t.opts.keepalive {
			t.curEvent = "keepalive"
			t.suppressKeepalive = true
			return
		}

		t.flushSuppressedDelta()
		t.curEvent = string(eventName)
		t.writeLine(line)
		return
	}

	if bytes.HasPrefix(trimmed, []byte("data:")) {
		if t.suppressDelta {
			t.addSuppressedDeltaLine(line)
			if seq, ok := extractSequenceNumberFromSSEDataLine(trimmed); ok {
				t.addSuppressedDeltaSeq(seq)
			}
			return
		}
		if t.suppressKeepalive {
			return
		}

		if !t.opts.maskInstructions && !t.opts.maskText {
			t.flushSuppressedDelta()
			t.writeLine(line)
			return
		}

		payload := bytes.TrimSpace(trimmed[len("data:"):])
		if (t.opts.maskInstructions && bytes.Contains(payload, []byte(`"instructions"`))) ||
			(t.opts.maskText && bytes.Contains(payload, []byte(`"text"`))) {
			masked, changed := maskJSONPayload(payload, t.opts.maskInstructions, t.opts.maskText)
			if changed {
				t.flushSuppressedDelta()
				out := make([]byte, 0, len(masked)+7)
				out = append(out, "data: "...)
				out = append(out, masked...)
				out = append(out, '\n')
				t.writeLine(out)
				return
			}
		}

		t.flushSuppressedDelta()
		t.writeLine(line)
		return
	}

	if t.suppressDelta {
		t.addSuppressedDeltaLine(line)
		return
	}
	if t.suppressKeepalive {
		return
	}

	t.flushSuppressedDelta()
	t.writeLine(line)
}

func (t *sseLogTeeReadCloser) addSuppressedDeltaLine(line []byte) {
	t.suppressedDelta.lines++
	t.suppressedDelta.bytes += int64(len(line))
}

func (t *sseLogTeeReadCloser) addSuppressedDeltaSeq(seq int64) {
	if !t.suppressedDelta.seqOK {
		t.suppressedDelta.seqOK = true
		t.suppressedDelta.seqMin = seq
		t.suppressedDelta.seqMax = seq
		return
	}
	if seq < t.suppressedDelta.seqMin {
		t.suppressedDelta.seqMin = seq
	}
	if seq > t.suppressedDelta.seqMax {
		t.suppressedDelta.seqMax = seq
	}
}

func (t *sseLogTeeReadCloser) flushSuppressedDelta() {
	if t.suppressedDelta.events <= 0 {
		return
	}
	sep := "=================\n"
	_, _ = t.w.Write([]byte(sep))

	line := fmt.Sprintf(
		"response.output_text.delta (suppressed events=%d lines=%d bytes=%d",
		t.suppressedDelta.events,
		t.suppressedDelta.lines,
		t.suppressedDelta.bytes,
	)
	if t.suppressedDelta.seqOK {
		line += fmt.Sprintf(" seq=%d..%d", t.suppressedDelta.seqMin, t.suppressedDelta.seqMax)
	}
	line += ")\n"
	_, _ = t.w.Write([]byte(line))
	_, _ = t.w.Write([]byte(sep))

	t.suppressedDelta = suppressedDeltaStats{}
}

func (t *sseLogTeeReadCloser) writeLine(line []byte) {
	_, _ = t.w.Write(line)
}

func extractSequenceNumberFromSSEDataLine(line []byte) (int64, bool) {
	// line is trimmed (no trailing newline), starts with "data:".
	idx := bytes.Index(line, []byte(`"sequence_number"`))
	if idx < 0 {
		return 0, false
	}
	rest := line[idx+len(`"sequence_number"`):]
	colon := bytes.IndexByte(rest, ':')
	if colon < 0 {
		return 0, false
	}
	i := idx + len(`"sequence_number"`) + colon + 1
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) {
		return 0, false
	}

	sign := int64(1)
	if line[i] == '-' {
		sign = -1
		i++
	}
	if i >= len(line) || line[i] < '0' || line[i] > '9' {
		return 0, false
	}

	var n int64
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		n = n*10 + int64(line[i]-'0')
		i++
	}
	return sign * n, true
}

func maskJSONPayload(payload []byte, maskInstructions bool, maskText bool) ([]byte, bool) {
	if len(payload) == 0 {
		return payload, false
	}

	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		return payload, false
	}
	changed := maskJSONValue(v, maskInstructions, maskText)
	if !changed {
		return payload, false
	}
	out, err := json.Marshal(v)
	if err != nil {
		return payload, false
	}
	return out, true
}

func maskJSONValue(v any, maskInstructions bool, maskText bool) bool {
	switch t := v.(type) {
	case map[string]any:
		changed := false
		for k, val := range t {
			if maskInstructions && k == "instructions" {
				if s, ok := val.(string); ok {
					t[k] = maskMiddleKeep6(s)
					changed = true
					continue
				}
			}
			if maskText && k == "text" {
				if s, ok := val.(string); ok {
					t[k] = maskMiddleKeep6(s)
					changed = true
					continue
				}
			}
			if maskJSONValue(val, maskInstructions, maskText) {
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for i := range t {
			if maskJSONValue(t[i], maskInstructions, maskText) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

func maskMiddleKeep6(s string) string {
	r := []rune(s)
	if len(r) <= 12 {
		return s
	}
	return string(r[:6]) + "***" + string(r[len(r)-6:])
}
