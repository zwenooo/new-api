package oai_cc

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	imageTokenAreaDivisor   = 750
	imageTokenLongEdgeLimit = 1568
	imageTokenCap           = 3278

	pdfTokensPerPage        = 2000
	pdfBytesPerPageEstimate = 200_000
	pdfPageLimit            = 100
)

func isNonWesternRune(r rune) bool {
	if r == 0 {
		return false
	}

	// Mirrors openai-claude-main/src/token-estimator.ts
	isWestern :=
		(r >= 0x0000 && r <= 0x007f) ||
			(r >= 0x0080 && r <= 0x00ff) ||
			(r >= 0x0100 && r <= 0x024f) ||
			(r >= 0x1e00 && r <= 0x1eff) ||
			(r >= 0x2c60 && r <= 0x2c7f) ||
			(r >= 0xa720 && r <= 0xa7ff) ||
			(r >= 0xab30 && r <= 0xab6f)

	return !isWestern
}

func countTokensText(text any) int {
	if text == nil {
		return 0
	}

	var s string
	switch t := text.(type) {
	case string:
		s = t
	default:
		s = fmt.Sprint(t)
	}
	if s == "" {
		return 0
	}

	charUnits := 0.0
	for _, r := range s {
		if isNonWesternRune(r) {
			charUnits += 4.0
		} else {
			charUnits += 1.0
		}
	}

	tokens := charUnits / 4.0

	var accTokens float64
	switch {
	case tokens < 100.0:
		accTokens = tokens * 1.5
	case tokens < 200.0:
		accTokens = tokens * 1.3
	case tokens < 300.0:
		accTokens = tokens * 1.25
	case tokens < 800.0:
		accTokens = tokens * 1.2
	default:
		accTokens = tokens * 1.0
	}

	if accTokens <= 0 {
		return 0
	}
	return int(math.Floor(accTokens))
}

func parsePositiveIntFromString(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}

	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
	}
	start := i
	for i < len(s) {
		ch := s[i]
		if ch < '0' || ch > '9' {
			break
		}
		i++
	}
	if i == start {
		return 0, false
	}

	n, err := strconv.Atoi(s[:i])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func toPositiveInt(value any) (int, bool) {
	if value == nil {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, v > 0
	case int64:
		maxInt := int64(^uint(0) >> 1)
		if v <= 0 || v > maxInt {
			return 0, false
		}
		return int(v), true
	case float64:
		// JSON numbers decode to float64; mimic JS parseInt(String(value)).
		return parsePositiveIntFromString(fmt.Sprint(v))
	case json.Number:
		return parsePositiveIntFromString(v.String())
	case string:
		return parsePositiveIntFromString(v)
	default:
		return parsePositiveIntFromString(fmt.Sprint(v))
	}
}

func stripWhitespace(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

type dataURLImagePayload struct {
	mediaType string
	data      string
}

func parseDataURLImagePayload(value any) *dataURLImagePayload {
	raw, ok := value.(string)
	if !ok {
		return nil
	}
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "data:") {
		return nil
	}

	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return nil
	}
	header := raw[len("data:"):comma]
	data := raw[comma+1:]

	if data == "" {
		return nil
	}

	parts := strings.Split(header, ";")
	if len(parts) == 0 {
		return nil
	}
	mediaType := strings.ToLower(strings.TrimSpace(parts[0]))
	if !strings.HasPrefix(mediaType, "image/") {
		return nil
	}

	isBase64 := false
	for _, p := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(p), "base64") {
			isBase64 = true
			break
		}
	}
	if !isBase64 {
		return nil
	}

	return &dataURLImagePayload{mediaType: mediaType, data: data}
}

func decodeBase64ToBytes(base64Text any) []byte {
	s, ok := base64Text.(string)
	if !ok {
		return nil
	}
	normalized := stripWhitespace(s)
	if normalized == "" {
		return nil
	}

	if b, err := base64.StdEncoding.DecodeString(normalized); err == nil && len(b) > 0 {
		return b
	}
	if b, err := base64.RawStdEncoding.DecodeString(normalized); err == nil && len(b) > 0 {
		return b
	}
	return nil
}

func parsePngDimensions(buf []byte) (int, int, bool) {
	if len(buf) < 24 {
		return 0, 0, false
	}
	pngSig := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	for i := 0; i < len(pngSig); i++ {
		if buf[i] != pngSig[i] {
			return 0, 0, false
		}
	}
	w := int(binary.BigEndian.Uint32(buf[16:20]))
	h := int(binary.BigEndian.Uint32(buf[20:24]))
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func parseGifDimensions(buf []byte) (int, int, bool) {
	if len(buf) < 10 {
		return 0, 0, false
	}
	sig := string(buf[0:6])
	if sig != "GIF87a" && sig != "GIF89a" {
		return 0, 0, false
	}
	w := int(binary.LittleEndian.Uint16(buf[6:8]))
	h := int(binary.LittleEndian.Uint16(buf[8:10]))
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func parseBmpDimensions(buf []byte) (int, int, bool) {
	if len(buf) < 26 {
		return 0, 0, false
	}
	if buf[0] != 0x42 || buf[1] != 0x4d {
		return 0, 0, false
	}
	w := int(int32(binary.LittleEndian.Uint32(buf[18:22])))
	h := int(int32(binary.LittleEndian.Uint32(buf[22:26])))
	if w < 0 {
		w = -w
	}
	if h < 0 {
		h = -h
	}
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func parseJpegDimensions(buf []byte) (int, int, bool) {
	if len(buf) < 4 {
		return 0, 0, false
	}
	if buf[0] != 0xff || buf[1] != 0xd8 {
		return 0, 0, false
	}

	offset := 2
	for offset+3 < len(buf) {
		for offset < len(buf) && buf[offset] != 0xff {
			offset++
		}
		if offset+1 >= len(buf) {
			break
		}

		for offset < len(buf) && buf[offset] == 0xff {
			offset++
		}
		if offset >= len(buf) {
			break
		}

		marker := buf[offset]
		offset++

		if marker == 0xd9 || marker == 0xda {
			break
		}
		if offset+1 >= len(buf) {
			break
		}

		segmentLength := int(binary.BigEndian.Uint16(buf[offset : offset+2]))
		if segmentLength < 2 {
			break
		}

		isSof := (marker >= 0xc0 && marker <= 0xc3) ||
			(marker >= 0xc5 && marker <= 0xc7) ||
			(marker >= 0xc9 && marker <= 0xcb) ||
			(marker >= 0xcd && marker <= 0xcf)

		if isSof {
			if offset+6 >= len(buf) {
				return 0, 0, false
			}
			h := int(binary.BigEndian.Uint16(buf[offset+3 : offset+5]))
			w := int(binary.BigEndian.Uint16(buf[offset+5 : offset+7]))
			if w <= 0 || h <= 0 {
				return 0, 0, false
			}
			return w, h, true
		}

		offset += segmentLength
	}

	return 0, 0, false
}

func parseWebpDimensions(buf []byte) (int, int, bool) {
	if len(buf) < 30 {
		return 0, 0, false
	}
	if string(buf[0:4]) != "RIFF" || string(buf[8:12]) != "WEBP" {
		return 0, 0, false
	}

	chunkType := string(buf[12:16])
	switch chunkType {
	case "VP8X":
		if len(buf) < 30 {
			return 0, 0, false
		}
		widthMinus1 := int(uint32(buf[24]) | uint32(buf[25])<<8 | uint32(buf[26])<<16)
		heightMinus1 := int(uint32(buf[27]) | uint32(buf[28])<<8 | uint32(buf[29])<<16)
		w := widthMinus1 + 1
		h := heightMinus1 + 1
		if w <= 0 || h <= 0 {
			return 0, 0, false
		}
		return w, h, true
	case "VP8 ":
		if len(buf) < 30 {
			return 0, 0, false
		}
		w := int(binary.LittleEndian.Uint16(buf[26:28]) & 0x3fff)
		h := int(binary.LittleEndian.Uint16(buf[28:30]) & 0x3fff)
		if w <= 0 || h <= 0 {
			return 0, 0, false
		}
		return w, h, true
	case "VP8L":
		if len(buf) < 25 || buf[20] != 0x2f {
			return 0, 0, false
		}
		bits := binary.LittleEndian.Uint32(buf[21:25])
		w := int(bits&0x3fff) + 1
		h := int((bits>>14)&0x3fff) + 1
		if w <= 0 || h <= 0 {
			return 0, 0, false
		}
		return w, h, true
	default:
		return 0, 0, false
	}
}

func parseImageDimensionsFromBuffer(buf []byte) (int, int, bool) {
	if w, h, ok := parsePngDimensions(buf); ok {
		return w, h, ok
	}
	if w, h, ok := parseJpegDimensions(buf); ok {
		return w, h, ok
	}
	if w, h, ok := parseGifDimensions(buf); ok {
		return w, h, ok
	}
	if w, h, ok := parseWebpDimensions(buf); ok {
		return w, h, ok
	}
	if w, h, ok := parseBmpDimensions(buf); ok {
		return w, h, ok
	}
	return 0, 0, false
}

func normalizeImageSizeForBilling(width int, height int) (int, int, bool) {
	w, okW := toPositiveInt(width)
	h, okH := toPositiveInt(height)
	if !okW || !okH {
		return 0, 0, false
	}

	longEdge := w
	if h > longEdge {
		longEdge = h
	}
	if longEdge <= imageTokenLongEdgeLimit {
		return w, h, true
	}

	scale := float64(imageTokenLongEdgeLimit) / float64(longEdge)
	if w >= h {
		newH := int(math.Round(float64(h) * scale))
		if newH < 1 {
			newH = 1
		}
		return imageTokenLongEdgeLimit, newH, true
	}

	newW := int(math.Round(float64(w) * scale))
	if newW < 1 {
		newW = 1
	}
	return newW, imageTokenLongEdgeLimit, true
}

func calculateImageTokensFromArea(width int, height int) int {
	w, h, ok := normalizeImageSizeForBilling(width, height)
	if !ok {
		return 0
	}
	area := w * h
	if area <= 0 {
		return 0
	}
	tokens := int(math.Round(float64(area) / float64(imageTokenAreaDivisor)))
	if tokens < 1 {
		tokens = 1
	}
	if tokens > imageTokenCap {
		tokens = imageTokenCap
	}
	return tokens
}

func isImageContentBlock(block any) bool {
	b, ok := block.(map[string]any)
	if !ok || b == nil {
		return false
	}
	typ := strings.ToLower(strings.TrimSpace(asString(b["type"])))
	if typ == "image" || typ == "input_image" || typ == "image_url" {
		return true
	}
	if _, ok := b["image_url"]; ok {
		return true
	}
	source, _ := b["source"].(map[string]any)
	mediaType := strings.ToLower(strings.TrimSpace(asString(source["media_type"])))
	if mediaType == "" {
		mediaType = strings.ToLower(strings.TrimSpace(asString(source["mediaType"])))
	}
	return strings.HasPrefix(mediaType, "image/")
}

func estimateImageTokensFromContentBlock(block any) int {
	if !isImageContentBlock(block) {
		return 0
	}
	b, _ := block.(map[string]any)

	wHint, okW := toPositiveInt(firstNonNil(b["width"], b["image_width"], getNested(b, "source", "width"), getNested(b, "image_url", "width")))
	hHint, okH := toPositiveInt(firstNonNil(b["height"], b["image_height"], getNested(b, "source", "height"), getNested(b, "image_url", "height")))
	if okW && okH {
		return calculateImageTokensFromArea(wHint, hHint)
	}

	var base64Data string
	if source, ok := b["source"].(map[string]any); ok && source != nil {
		if strings.ToLower(strings.TrimSpace(asString(source["type"]))) == "base64" {
			if s, ok := source["data"].(string); ok && s != "" {
				base64Data = s
			}
		}
		if base64Data == "" {
			if s, ok := source["data"].(string); ok && s != "" {
				base64Data = s
			}
		}
		if base64Data == "" {
			if u, ok := source["url"].(string); ok && u != "" {
				if parsed := parseDataURLImagePayload(u); parsed != nil && parsed.data != "" {
					base64Data = parsed.data
				}
			}
		}
	}

	if base64Data == "" {
		imageURL := b["image_url"]
		if s, ok := imageURL.(string); ok && s != "" {
			if parsed := parseDataURLImagePayload(s); parsed != nil && parsed.data != "" {
				base64Data = parsed.data
			}
		} else if m, ok := imageURL.(map[string]any); ok && m != nil {
			if u, ok := m["url"].(string); ok && u != "" {
				if parsed := parseDataURLImagePayload(u); parsed != nil && parsed.data != "" {
					base64Data = parsed.data
				}
			}
		}
	}

	if base64Data == "" {
		return 0
	}

	buf := decodeBase64ToBytes(base64Data)
	if len(buf) == 0 {
		return 0
	}

	w, h, ok := parseImageDimensionsFromBuffer(buf)
	if !ok {
		return 0
	}
	return calculateImageTokensFromArea(w, h)
}

func estimatePdfTokensFromDocumentBlock(block map[string]any) int {
	titleTokens := 0
	if t, ok := block["title"].(string); ok && t != "" {
		titleTokens = countTokensText(t)
	}
	contextTokens := 0
	if t, ok := block["context"].(string); ok && t != "" {
		contextTokens = countTokensText(t)
	}

	source, _ := block["source"].(map[string]any)
	mediaType := strings.ToLower(strings.TrimSpace(asString(source["media_type"])))
	if mediaType == "" {
		mediaType = strings.ToLower(strings.TrimSpace(asString(source["mediaType"])))
	}
	isPdf := mediaType == "application/pdf" || strings.HasSuffix(mediaType, "/pdf")
	if !isPdf {
		return titleTokens + contextTokens
	}

	pagesEstimate := 1
	if s, ok := source["data"].(string); ok && s != "" {
		norm := stripWhitespace(s)
		padding := 0
		if strings.HasSuffix(norm, "==") {
			padding = 2
		} else if strings.HasSuffix(norm, "=") {
			padding = 1
		}
		bytesApprox := int(math.Floor(float64(len(norm))*3.0/4.0)) - padding
		if bytesApprox < 0 {
			bytesApprox = 0
		}
		pagesEstimate = int(math.Round(float64(bytesApprox) / float64(pdfBytesPerPageEstimate)))
		if pagesEstimate < 1 {
			pagesEstimate = 1
		}
	} else if u, ok := source["url"].(string); ok && u != "" {
		pagesEstimate = 1
	}

	if pagesEstimate < 1 {
		pagesEstimate = 1
	}
	if pagesEstimate > pdfPageLimit {
		pagesEstimate = pdfPageLimit
	}

	pdfTokens := pagesEstimate * pdfTokensPerPage
	return titleTokens + contextTokens + pdfTokens
}

func truncateStringsForJSON(value any, maxLen int) any {
	switch v := value.(type) {
	case string:
		if maxLen > 0 && len(v) > maxLen {
			return v[:maxLen] + "…"
		}
		return v
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, vv := range v {
			out[k] = truncateStringsForJSON(vv, maxLen)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = truncateStringsForJSON(v[i], maxLen)
		}
		return out
	default:
		return value
	}
}

func safeJsonStringify(value any, maxLen int) string {
	truncated := truncateStringsForJSON(value, 4096)
	b, err := json.Marshal(truncated)
	if err != nil {
		return ""
	}
	s := string(b)
	if maxLen > 0 && len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}

func countToolTokensForUsage(tool any) int {
	t, ok := tool.(map[string]any)
	if !ok || t == nil {
		return 0
	}
	total := 0
	if name, ok := t["name"].(string); ok {
		total += countTokensText(name)
	}
	if desc, ok := t["description"].(string); ok {
		total += countTokensText(desc)
	}
	if schema, ok := t["input_schema"]; ok {
		total += countTokensText(safeJsonStringify(schema, 40000))
	}
	return total
}

func countMessageBlockTokensForUsage(block any) int {
	if block == nil {
		return 0
	}
	if s, ok := block.(string); ok {
		return countTokensText(s)
	}
	b, ok := block.(map[string]any)
	if !ok || b == nil {
		return 0
	}

	typ := asString(b["type"])

	imageTokens := estimateImageTokensFromContentBlock(b)
	if imageTokens > 0 || isImageContentBlock(b) {
		return imageTokens
	}

	if typ == "document" {
		return estimatePdfTokensFromDocumentBlock(b)
	}

	if typ == "text" {
		if text, ok := b["text"].(string); ok {
			return countTokensText(text)
		}
	}

	if typ == "thinking" {
		if thinking, ok := b["thinking"].(string); ok {
			return countTokensText(thinking)
		}
	}

	if typ == "compaction" {
		if content, ok := b["content"].(string); ok {
			return countTokensText(content)
		}
	}

	if typ == "tool_use" {
		total := 0
		if name, ok := b["name"].(string); ok {
			total += countTokensText(name)
		}
		total += countTokensText(safeJsonStringify(b["input"], 20000))
		return total
	}

	if typ == "tool_result" {
		c := b["content"]
		switch vv := c.(type) {
		case string:
			return countTokensText(vv)
		case []any:
			return countMessageContentTokens(vv)
		default:
			return countTokensText(safeJsonStringify(vv, 20000))
		}
	}

	if text, ok := b["text"].(string); ok {
		return countTokensText(text)
	}

	if data, ok := b["data"].(string); ok && data != "" {
		if len(data) > 8192 {
			data = data[:8192]
		}
		return countTokensText(data)
	}

	return countTokensText(safeJsonStringify(block, 20000))
}

func countMessageContentTokens(content any) int {
	if content == nil {
		return 0
	}
	if s, ok := content.(string); ok {
		return countTokensText(s)
	}
	arr, ok := content.([]any)
	if !ok {
		return 0
	}
	total := 0
	for _, block := range arr {
		total += countMessageBlockTokensForUsage(block)
	}
	return total
}

func countSystemTokens(system any) int {
	if system == nil {
		return 0
	}
	if s, ok := system.(string); ok {
		return countTokensText(s)
	}
	arr, ok := system.([]any)
	if !ok {
		return 0
	}
	total := 0
	for _, item := range arr {
		if s, ok := item.(string); ok {
			total += countTokensText(s)
			continue
		}
		m, ok := item.(map[string]any)
		if !ok || m == nil {
			continue
		}
		if text, ok := m["text"].(string); ok {
			total += countTokensText(text)
		}
	}
	return total
}

func countToolsTokens(tools any) int {
	arr, ok := tools.([]any)
	if !ok {
		return 0
	}
	total := 0
	for _, tool := range arr {
		total += countToolTokensForUsage(tool)
	}
	return total
}

func countInputTokensLocal(anthropicReq map[string]any) int {
	req := anthropicReq
	if req == nil {
		req = map[string]any{}
	}

	total := 0
	total += countSystemTokens(req["system"])

	if messages, ok := req["messages"].([]any); ok {
		for _, msgAny := range messages {
			msg, ok := msgAny.(map[string]any)
			if !ok || msg == nil {
				continue
			}
			total += countMessageContentTokens(msg["content"])
		}
	}

	total += countToolsTokens(req["tools"])

	if total < 1 {
		total = 1
	}
	return total
}
