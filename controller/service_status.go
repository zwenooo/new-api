package controller

import (
	"bytes"
	"fmt"
	"html"
	"image"
	"image/color"
	imagedraw "image/draw"
	"image/png"
	"net/http"
	"one-api/common"
	"one-api/model"
	"one-api/setting/operation_setting"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	serviceStatusBucketNoData   = 0
	serviceStatusBucketUp       = 1
	serviceStatusBucketDegraded = 2
	serviceStatusBucketDown     = 3

	serviceStatusImagePaddingX = 24
	serviceStatusImagePaddingY = 20
	serviceStatusImageLabelW   = 260
	serviceStatusImageHeaderH  = 56
	serviceStatusImageRowH     = 42
	serviceStatusImageCellW    = 7
	serviceStatusImageCellH    = 14
	serviceStatusImageCellGap  = 2
)

var (
	serviceStatusImageFontOnce sync.Once
	serviceStatusImageFont     *opentype.Font
	serviceStatusImageFontErr  error
)

var serviceStatusImageFontCandidates = []string{
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/opentype/noto/NotoSansCJKSC-Regular.otf",
	"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
	"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
	"/usr/share/fonts/truetype/arphic/uming.ttc",
	"/mnt/c/Windows/Fonts/msyh.ttc",
	"/mnt/c/Windows/Fonts/msyhbd.ttc",
	"/mnt/c/Windows/Fonts/simhei.ttf",
	"/mnt/c/Windows/Fonts/simsun.ttc",
}

type serviceStatusOverall struct {
	Uptime          float64 `json:"uptime"`
	AvailableBucket int     `json:"available_buckets"`
	DataBucket      int     `json:"data_buckets"`
}

type serviceStatusTimelineGroup struct {
	Code            string  `json:"code"`
	DisplayName     string  `json:"display_name"`
	Description     string  `json:"description"`
	Uptime          float64 `json:"uptime"`
	AvailableBucket int     `json:"available_buckets"`
	DataBucket      int     `json:"data_buckets"`

	Statuses     []int   `json:"statuses"`
	Success      []int64 `json:"success"`
	ServerErrors []int64 `json:"server_errors"`
	ClientAborts []int64 `json:"client_aborts"`
}

type serviceStatusTimelineResp struct {
	Start           int64                        `json:"start"`
	End             int64                        `json:"end"`
	Bucket          string                       `json:"bucket"`
	BucketSeconds   int64                        `json:"bucket_seconds"`
	BucketStarts    []int64                      `json:"bucket_starts"`
	BackfillPending bool                         `json:"backfill_pending"`
	BackfilledStart int64                        `json:"backfilled_start"`
	Overall         serviceStatusOverall         `json:"overall"`
	Groups          []serviceStatusTimelineGroup `json:"groups"`
}

func buildServiceStatusTimelineData(c *gin.Context) (*serviceStatusTimelineResp, int, string, error) {
	nowTime := time.Now()
	now := nowTime.Unix()

	bucket := strings.TrimSpace(c.Query("bucket"))
	if bucket == "" {
		bucket = strings.TrimSpace(operation_setting.GetMonitorSetting().ServiceStatusDefaultBucket)
	}
	if bucket == "" {
		return nil, http.StatusInternalServerError, "monitor_setting.service_status_default_bucket 配置无效", nil
	}

	var bucketSeconds int64
	switch bucket {
	case "minute":
		bucketSeconds = 60
	case "hour":
		bucketSeconds = 3600
	case "day":
		bucketSeconds = 86400
	default:
		return nil, http.StatusBadRequest, "bucket 无效，仅支持 minute/hour/day", nil
	}

	end, present, err := parseUnixSecondsQuery(c, "end")
	if err != nil {
		return nil, http.StatusBadRequest, "end 无效", nil
	}
	if !present {
		if bucket == "day" {
			end = time.Date(nowTime.Year(), nowTime.Month(), nowTime.Day(), 0, 0, 0, 0, nowTime.Location()).
				Add(24 * time.Hour).
				Unix()
		} else {
			end = now
		}
	}

	start, present, err := parseUnixSecondsQuery(c, "start")
	if err != nil {
		return nil, http.StatusBadRequest, "start 无效", nil
	}
	if !present {
		defaultRangeMinutes := operation_setting.GetMonitorSetting().ServiceStatusDefaultRangeMinutes
		if defaultRangeMinutes <= 0 {
			defaultRangeDays := operation_setting.GetMonitorSetting().ServiceStatusDefaultRangeDays
			if defaultRangeDays <= 0 {
				return nil, http.StatusInternalServerError, "monitor_setting.service_status_default_range_minutes/service_status_default_range_days 配置无效", nil
			}
			defaultRangeMinutes = defaultRangeDays * 1440
		}
		start = end - int64(defaultRangeMinutes)*60
	}

	if start <= 0 || end <= 0 || start >= end {
		return nil, http.StatusBadRequest, "start/end 无效", nil
	}

	const maxRangeSeconds = 180 * 86400
	if end-start > maxRangeSeconds {
		return nil, http.StatusBadRequest, "时间范围过大，请缩小到 180 天以内", nil
	}

	startBucket := start - start%bucketSeconds
	endBucket := (end - 1) - (end-1)%bucketSeconds
	if endBucket < startBucket {
		return nil, http.StatusBadRequest, "时间范围无可用 bucket", nil
	}

	bucketCount := int((endBucket-startBucket)/bucketSeconds) + 1
	if bucketCount > 4000 {
		return nil, http.StatusBadRequest, "bucket 数量过多，请增大粒度或缩小时间范围", nil
	}

	bucketStarts := make([]int64, 0, bucketCount)
	bucketIndex := make(map[int64]int, bucketCount)
	for t := startBucket; t <= endBucket; t += bucketSeconds {
		bucketIndex[t] = len(bucketStarts)
		bucketStarts = append(bucketStarts, t)
	}

	groups, err := model.ListGroups(nil)
	if err != nil {
		return nil, 0, "", err
	}

	enabledGroups := make([]model.Group, 0, len(groups))
	for _, g := range groups {
		// 服务状态图面向最终用户/机器人发送场景，只展示启用且用户可见的分组。
		if g.Enabled && g.UserSelectable {
			enabledGroups = append(enabledGroups, g)
		}
	}
	if len(enabledGroups) == 0 {
		resp := &serviceStatusTimelineResp{
			Start:         start,
			End:           end,
			Bucket:        bucket,
			BucketSeconds: bucketSeconds,
			BucketStarts:  bucketStarts,
			Overall:       serviceStatusOverall{},
			Groups:        []serviceStatusTimelineGroup{},
		}
		return resp, 0, "", nil
	}

	sort.SliceStable(enabledGroups, func(i, j int) bool {
		return enabledGroups[i].Id < enabledGroups[j].Id
	})

	groupCodes := make([]string, 0, len(enabledGroups))
	groupByCode := make(map[string]model.Group, len(enabledGroups))
	for _, g := range enabledGroups {
		if g.Id <= 0 {
			continue
		}
		groupIDStr := strconv.Itoa(g.Id)
		groupCodes = append(groupCodes, groupIDStr)
		groupByCode[groupIDStr] = g
	}

	sourceBucketSeconds := bucketSeconds
	if bucket == "hour" {
		sourceBucketSeconds = 60
	} else if bucket == "day" {
		sourceBucketSeconds = 3600
	}

	readiness, err := model.PrepareServiceStatusBucketStatsForRead(c.Request.Context(), startBucket, endBucket+bucketSeconds, sourceBucketSeconds)
	if err != nil {
		return nil, 0, "", err
	}
	if readiness.Pending {
		model.RequestServiceStatusBucketStatsBackfill(startBucket, endBucket+bucketSeconds, sourceBucketSeconds)
	}

	statRows, err := model.ListServiceStatusBucketStats(c.Request.Context(), startBucket, endBucket+bucketSeconds, sourceBucketSeconds, groupCodes)
	if err != nil {
		return nil, 0, "", err
	}

	successByGroupBucket := make(map[string]map[int64]int64, len(groupCodes))
	serverErrByGroupBucket := make(map[string]map[int64]int64, len(groupCodes))
	clientAbortByGroupBucket := make(map[string]map[int64]int64, len(groupCodes))

	for _, row := range statRows {
		gc := strings.TrimSpace(row.GroupCode)
		if gc == "" {
			continue
		}
		if _, ok := groupByCode[gc]; !ok {
			continue
		}
		targetBucketStart := row.BucketStart - row.BucketStart%bucketSeconds
		if targetBucketStart <= 0 {
			continue
		}
		if _, ok := successByGroupBucket[gc]; !ok {
			successByGroupBucket[gc] = make(map[int64]int64)
		}
		if _, ok := serverErrByGroupBucket[gc]; !ok {
			serverErrByGroupBucket[gc] = make(map[int64]int64)
		}
		if _, ok := clientAbortByGroupBucket[gc]; !ok {
			clientAbortByGroupBucket[gc] = make(map[int64]int64)
		}
		successByGroupBucket[gc][targetBucketStart] += row.Success
		serverErrByGroupBucket[gc][targetBucketStart] += row.ServerErrors
		clientAbortByGroupBucket[gc][targetBucketStart] += row.ClientAborts
	}

	respGroups := make([]serviceStatusTimelineGroup, 0, len(enabledGroups))
	overallDataBuckets := 0
	overallAvailableBuckets := 0
	overallSuccessTotal := int64(0)
	overallRequestTotal := int64(0)

	for _, g := range enabledGroups {
		if g.Id <= 0 {
			continue
		}
		groupIDStr := strconv.Itoa(g.Id)
		displayName := strings.TrimSpace(g.DisplayName)
		if displayName == "" {
			displayName = strings.TrimSpace(g.Code)
			if displayName == "" {
				displayName = groupIDStr
			}
		}

		statuses := make([]int, len(bucketStarts))
		successArr := make([]int64, len(bucketStarts))
		serverErrArr := make([]int64, len(bucketStarts))
		clientAbortArr := make([]int64, len(bucketStarts))

		dataBuckets := 0
		availableBuckets := 0
		successTotal := int64(0)
		requestTotal := int64(0)

		for _, t := range bucketStarts {
			idx := bucketIndex[t]
			success := int64(0)
			if m, ok := successByGroupBucket[groupIDStr]; ok {
				success = m[t]
			}
			serverErr := int64(0)
			if m, ok := serverErrByGroupBucket[groupIDStr]; ok {
				serverErr = m[t]
			}
			clientAbort := int64(0)
			if m, ok := clientAbortByGroupBucket[groupIDStr]; ok {
				clientAbort = m[t]
			}

			successArr[idx] = success
			serverErrArr[idx] = serverErr
			clientAbortArr[idx] = clientAbort

			hasData := success > 0 || serverErr > 0
			if hasData {
				dataBuckets++
				successTotal += success
				requestTotal += success + serverErr
			}

			switch {
			case success > 0 && serverErr == 0:
				statuses[idx] = serviceStatusBucketUp
				availableBuckets++
			case success > 0 && serverErr > 0:
				statuses[idx] = serviceStatusBucketDegraded
				availableBuckets++
			case success == 0 && serverErr > 0:
				statuses[idx] = serviceStatusBucketDown
			default:
				statuses[idx] = serviceStatusBucketNoData
			}
		}

		uptime := 0.0
		if requestTotal > 0 {
			uptime = float64(successTotal) / float64(requestTotal)
		}

		respGroups = append(respGroups, serviceStatusTimelineGroup{
			Code:            strings.TrimSpace(g.Code),
			DisplayName:     displayName,
			Description:     strings.TrimSpace(g.Description),
			Uptime:          uptime,
			AvailableBucket: availableBuckets,
			DataBucket:      dataBuckets,
			Statuses:        statuses,
			Success:         successArr,
			ServerErrors:    serverErrArr,
			ClientAborts:    clientAbortArr,
		})

		overallDataBuckets += dataBuckets
		overallAvailableBuckets += availableBuckets
		overallSuccessTotal += successTotal
		overallRequestTotal += requestTotal
	}

	overallUptime := 0.0
	if overallRequestTotal > 0 {
		overallUptime = float64(overallSuccessTotal) / float64(overallRequestTotal)
	}

	resp := &serviceStatusTimelineResp{
		Start:           start,
		End:             end,
		Bucket:          bucket,
		BucketSeconds:   bucketSeconds,
		BucketStarts:    bucketStarts,
		BackfillPending: readiness.Pending,
		BackfilledStart: readiness.BackfilledStart,
		Overall: serviceStatusOverall{
			Uptime:          overallUptime,
			AvailableBucket: overallAvailableBuckets,
			DataBucket:      overallDataBuckets,
		},
		Groups: respGroups,
	}
	return resp, 0, "", nil
}

func serviceStatusImagePercent(value float64) string {
	return fmt.Sprintf("%.2f%%", value*100)
}

func serviceStatusImageBucketLabel(ts int64, bucket string) string {
	d := time.Unix(ts, 0)
	switch bucket {
	case "minute":
		return d.Format("15:04")
	case "hour":
		return d.Format("01-02 15:00")
	default:
		return d.Format("01-02")
	}
}

func serviceStatusImageClamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func serviceStatusImageCellColor(status int, serverErrorRate float64) (string, float64) {
	if status == serviceStatusBucketNoData {
		return "#94a3b8", 0.35
	}
	clamped := serviceStatusImageClamp01(serverErrorRate)
	hue := 140.0 * (1 - clamped*clamped)
	return fmt.Sprintf("hsl(%.0f, 82%%, 45%%)", hue), 1
}

func serviceStatusImageHSLToNRGBA(h, s, l float64) color.NRGBA {
	c := (1 - absFloat64(2*l-1)) * s
	x := c * (1 - absFloat64(modFloat64(h/60, 2)-1))
	m := l - c/2

	var r1, g1, b1 float64
	switch {
	case h < 60:
		r1, g1, b1 = c, x, 0
	case h < 120:
		r1, g1, b1 = x, c, 0
	case h < 180:
		r1, g1, b1 = 0, c, x
	case h < 240:
		r1, g1, b1 = 0, x, c
	case h < 300:
		r1, g1, b1 = x, 0, c
	default:
		r1, g1, b1 = c, 0, x
	}

	return color.NRGBA{
		R: uint8((r1 + m) * 255),
		G: uint8((g1 + m) * 255),
		B: uint8((b1 + m) * 255),
		A: 255,
	}
}

func modFloat64(a, b float64) float64 {
	return a - float64(int(a/b))*b
}

func absFloat64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func serviceStatusImageCellNRGBA(status int, serverErrorRate float64) (color.NRGBA, float64) {
	if status == serviceStatusBucketNoData {
		return color.NRGBA{R: 148, G: 163, B: 184, A: 255}, 0.35
	}
	clamped := serviceStatusImageClamp01(serverErrorRate)
	hue := 140.0 * (1 - clamped*clamped)
	return serviceStatusImageHSLToNRGBA(hue, 0.82, 0.45), 1
}

func serviceStatusImageTruncate(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	if maxRunes <= 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

func loadServiceStatusImageFont() (*opentype.Font, error) {
	serviceStatusImageFontOnce.Do(func() {
		var lastErr error
		for _, path := range serviceStatusImageFontCandidates {
			data, err := os.ReadFile(path)
			if err != nil {
				lastErr = err
				continue
			}
			collection, err := opentype.ParseCollection(data)
			if err != nil {
				lastErr = err
				continue
			}
			for i := 0; i < collection.NumFonts(); i++ {
				parsedFont, err := collection.Font(i)
				if err == nil {
					serviceStatusImageFont = parsedFont
					serviceStatusImageFontErr = nil
					return
				}
				lastErr = err
			}
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("未找到可用中文字体")
		}
		serviceStatusImageFontErr = fmt.Errorf("未找到可用中文字体，无法生成 PNG 状态图: %w", lastErr)
	})
	if serviceStatusImageFontErr != nil {
		return nil, serviceStatusImageFontErr
	}
	return serviceStatusImageFont, nil
}

func newServiceStatusImageFace(size float64) (font.Face, error) {
	loadedFont, err := loadServiceStatusImageFont()
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(loadedFont, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func serviceStatusImageFillRect(dst *image.RGBA, x, y, w, h int, fill color.NRGBA) {
	imagedraw.Draw(dst, image.Rect(x, y, x+w, y+h), &image.Uniform{C: fill}, image.Point{}, imagedraw.Over)
}

func serviceStatusImageDrawText(dst *image.RGBA, x, y int, face font.Face, fill color.Color, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	drawer := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(fill),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	drawer.DrawString(text)
}

func serviceStatusImageDrawHLine(dst *image.RGBA, x1, x2, y int, fill color.NRGBA) {
	if x2 <= x1 {
		return
	}
	serviceStatusImageFillRect(dst, x1, y, x2-x1, 1, fill)
}

func renderServiceStatusTimelinePNG(data *serviceStatusTimelineResp) ([]byte, error) {
	titleFace, err := newServiceStatusImageFace(14)
	if err != nil {
		return nil, err
	}
	defer titleFace.Close()

	subtitleFace, err := newServiceStatusImageFace(11)
	if err != nil {
		return nil, err
	}
	defer subtitleFace.Close()

	headerFace, err := newServiceStatusImageFace(12)
	if err != nil {
		return nil, err
	}
	defer headerFace.Close()

	bucketCount := len(data.BucketStarts)
	groupCount := len(data.Groups)
	timelineW := 0
	if bucketCount > 0 {
		timelineW = bucketCount*serviceStatusImageCellW + (bucketCount-1)*serviceStatusImageCellGap
	}
	headerW := serviceStatusImageLabelW + 20 + timelineW
	canvasW := serviceStatusImagePaddingX*2 + headerW
	if canvasW < 720 {
		canvasW = 720
	}
	contentH := serviceStatusImageHeaderH + groupCount*serviceStatusImageRowH
	if groupCount == 0 {
		contentH = serviceStatusImageHeaderH + 88
	}
	canvasH := serviceStatusImagePaddingY*2 + contentH

	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	serviceStatusImageFillRect(img, 0, 0, canvasW, canvasH, color.NRGBA{R: 248, G: 244, B: 237, A: 255})
	serviceStatusImageFillRect(img, serviceStatusImagePaddingX, serviceStatusImagePaddingY, headerW, contentH, color.NRGBA{R: 255, G: 251, B: 244, A: 235})

	headerX := serviceStatusImagePaddingX + serviceStatusImageLabelW + 20
	headerY := serviceStatusImagePaddingY + 18
	textMuted := color.NRGBA{R: 109, G: 96, B: 86, A: 255}
	textSubtle := color.NRGBA{R: 143, G: 131, B: 120, A: 255}
	textStrong := color.NRGBA{R: 30, G: 23, B: 18, A: 255}
	lineFill := color.NRGBA{R: 30, G: 23, B: 18, A: 18}

	serviceStatusImageDrawText(img, serviceStatusImagePaddingX+10, headerY, headerFace, textMuted, "分组")
	serviceStatusImageDrawText(img, headerX, headerY, headerFace, textMuted, "时间轴")

	if bucketCount > 0 {
		labelStep := 1
		if bucketCount > 8 {
			labelStep = (bucketCount + 7) / 8
		}
		for idx, ts := range data.BucketStarts {
			if idx%labelStep != 0 && idx != bucketCount-1 {
				continue
			}
			x := headerX + idx*(serviceStatusImageCellW+serviceStatusImageCellGap)
			serviceStatusImageDrawText(img, x, headerY+18, subtitleFace, textSubtle, serviceStatusImageBucketLabel(ts, data.Bucket))
		}
	}

	rowStartY := serviceStatusImagePaddingY + serviceStatusImageHeaderH
	if groupCount == 0 {
		serviceStatusImageDrawText(img, serviceStatusImagePaddingX+16, rowStartY+36, titleFace, textMuted, "暂无监控数据")
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	for groupIdx, group := range data.Groups {
		rowY := rowStartY + groupIdx*serviceStatusImageRowH
		centerY := rowY + serviceStatusImageRowH/2
		if groupIdx > 0 {
			serviceStatusImageDrawHLine(img, serviceStatusImagePaddingX+10, serviceStatusImagePaddingX+headerW-10, rowY, lineFill)
		}

		fullDisplayName := group.DisplayName
		if strings.TrimSpace(fullDisplayName) == "" {
			fullDisplayName = group.Code
		}
		displayName := serviceStatusImageTruncate(fullDisplayName, 22)
		uptimeText := "—"
		if group.DataBucket > 0 {
			uptimeText = serviceStatusImagePercent(group.Uptime)
		}
		subtitle := fmt.Sprintf("可用率 %s", uptimeText)
		if group.DataBucket > 0 {
			subtitle = fmt.Sprintf("%s · %d/%d", subtitle, group.AvailableBucket, group.DataBucket)
		}

		serviceStatusImageDrawText(img, serviceStatusImagePaddingX+10, rowY+16, titleFace, textStrong, displayName)
		serviceStatusImageDrawText(img, serviceStatusImagePaddingX+10, rowY+31, subtitleFace, textMuted, subtitle)

		for bucketIdx := range data.BucketStarts {
			x := headerX + bucketIdx*(serviceStatusImageCellW+serviceStatusImageCellGap)
			y := centerY - serviceStatusImageCellH/2
			status := serviceStatusBucketNoData
			if bucketIdx < len(group.Statuses) {
				status = group.Statuses[bucketIdx]
			}
			success := int64(0)
			if bucketIdx < len(group.Success) {
				success = group.Success[bucketIdx]
			}
			serverErr := int64(0)
			if bucketIdx < len(group.ServerErrors) {
				serverErr = group.ServerErrors[bucketIdx]
			}
			total := success + serverErr
			serverErrorRate := 0.0
			if total > 0 {
				serverErrorRate = float64(serverErr) / float64(total)
			}
			cellFill, opacity := serviceStatusImageCellNRGBA(status, serverErrorRate)
			cellFill.A = uint8(opacity * 255)
			serviceStatusImageFillRect(img, x, y, serviceStatusImageCellW, serviceStatusImageCellH, cellFill)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderServiceStatusTimelineSVG(data *serviceStatusTimelineResp) string {
	bucketCount := len(data.BucketStarts)
	groupCount := len(data.Groups)
	timelineW := 0
	if bucketCount > 0 {
		timelineW = bucketCount*serviceStatusImageCellW + (bucketCount-1)*serviceStatusImageCellGap
	}
	headerW := serviceStatusImageLabelW + 20 + timelineW
	canvasW := serviceStatusImagePaddingX*2 + headerW
	if canvasW < 720 {
		canvasW = 720
	}
	contentH := serviceStatusImageHeaderH + groupCount*serviceStatusImageRowH
	if groupCount == 0 {
		contentH = serviceStatusImageHeaderH + 88
	}
	canvasH := serviceStatusImagePaddingY*2 + contentH

	var b strings.Builder
	b.Grow(canvasW*4 + canvasH*8)
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" fill="none">`, canvasW, canvasH, canvasW, canvasH))
	b.WriteString(`<rect width="100%" height="100%" fill="#f8f4ed"/>`)
	b.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="24" fill="#fffbf4" fill-opacity="0.92" stroke="#1e1712" stroke-opacity="0.08"/>`, serviceStatusImagePaddingX, serviceStatusImagePaddingY, headerW, contentH))

	headerX := serviceStatusImagePaddingX + serviceStatusImageLabelW + 20
	headerY := serviceStatusImagePaddingY + 18
	b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#6d6056" font-size="12" font-family="Avenir Next, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif">分组</text>`, serviceStatusImagePaddingX+10, headerY))
	b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#6d6056" font-size="12" font-family="Avenir Next, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif">时间轴</text>`, headerX, headerY))

	if bucketCount > 0 {
		labelStep := 1
		if bucketCount > 8 {
			labelStep = (bucketCount + 7) / 8
		}
		for idx, ts := range data.BucketStarts {
			if idx%labelStep != 0 && idx != bucketCount-1 {
				continue
			}
			x := headerX + idx*(serviceStatusImageCellW+serviceStatusImageCellGap)
			label := html.EscapeString(serviceStatusImageBucketLabel(ts, data.Bucket))
			b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#8f8378" font-size="10" font-family="Avenir Next, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif">%s</text>`, x, headerY+18, label))
		}
	}

	rowStartY := serviceStatusImagePaddingY + serviceStatusImageHeaderH
	if groupCount == 0 {
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#6d6056" font-size="14" font-family="Avenir Next, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif">暂无监控数据</text>`, serviceStatusImagePaddingX+16, rowStartY+36))
		b.WriteString(`</svg>`)
		return b.String()
	}

	for groupIdx, group := range data.Groups {
		rowY := rowStartY + groupIdx*serviceStatusImageRowH
		centerY := rowY + serviceStatusImageRowH/2
		if groupIdx > 0 {
			b.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#1e1712" stroke-opacity="0.07"/>`, serviceStatusImagePaddingX+10, rowY, serviceStatusImagePaddingX+headerW-10, rowY))
		}

		fullDisplayName := group.DisplayName
		if strings.TrimSpace(fullDisplayName) == "" {
			fullDisplayName = group.Code
		}
		displayName := serviceStatusImageTruncate(fullDisplayName, 22)
		uptimeText := "—"
		if group.DataBucket > 0 {
			uptimeText = serviceStatusImagePercent(group.Uptime)
		}
		subtitle := fmt.Sprintf("可用率 %s", uptimeText)
		if group.DataBucket > 0 {
			subtitle = fmt.Sprintf("%s · %d/%d", subtitle, group.AvailableBucket, group.DataBucket)
		}

		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#1e1712" font-size="14" font-weight="700" font-family="Avenir Next, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif">%s</text>`, serviceStatusImagePaddingX+10, rowY+16, html.EscapeString(displayName)))
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#6d6056" font-size="11" font-family="Avenir Next, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif">%s</text>`, serviceStatusImagePaddingX+10, rowY+31, html.EscapeString(subtitle)))

		for bucketIdx, ts := range data.BucketStarts {
			x := headerX + bucketIdx*(serviceStatusImageCellW+serviceStatusImageCellGap)
			y := centerY - serviceStatusImageCellH/2
			status := serviceStatusBucketNoData
			if bucketIdx < len(group.Statuses) {
				status = group.Statuses[bucketIdx]
			}
			success := int64(0)
			if bucketIdx < len(group.Success) {
				success = group.Success[bucketIdx]
			}
			serverErr := int64(0)
			if bucketIdx < len(group.ServerErrors) {
				serverErr = group.ServerErrors[bucketIdx]
			}
			total := success + serverErr
			serverErrorRate := 0.0
			if total > 0 {
				serverErrorRate = float64(serverErr) / float64(total)
			}
			fill, opacity := serviceStatusImageCellColor(status, serverErrorRate)
			title := fmt.Sprintf("%s | %s | 可用率 %s", fullDisplayName, serviceStatusImageBucketLabel(ts, data.Bucket), uptimeText)
			if total > 0 {
				title = fmt.Sprintf("%s | %s | 可用率 %s", fullDisplayName, serviceStatusImageBucketLabel(ts, data.Bucket), serviceStatusImagePercent(float64(success)/float64(total)))
			}
			b.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="3" fill="%s" fill-opacity="%.2f" stroke="#0f172a" stroke-opacity="0.06">`, x, y, serviceStatusImageCellW, serviceStatusImageCellH, fill, opacity))
			b.WriteString(fmt.Sprintf(`<title>%s</title></rect>`, html.EscapeString(title)))
		}
	}

	b.WriteString(`</svg>`)
	return b.String()
}

func parseUnixSecondsQuery(c *gin.Context, key string) (value int64, present bool, err error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return 0, true, strconv.ErrSyntax
	}
	return v, true, nil
}

func GetServiceStatusTimeline(c *gin.Context) {
	data, statusCode, message, err := buildServiceStatusTimelineData(c)
	if message != "" {
		c.JSON(statusCode, gin.H{"success": false, "message": message})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func GetServiceStatusTimelineSVG(c *gin.Context) {
	data, statusCode, message, err := buildServiceStatusTimelineData(c)
	if message != "" {
		c.String(statusCode, message)
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	svg := renderServiceStatusTimelineSVG(data)
	c.Header("Content-Disposition", `inline; filename="service-status.svg"`)
	c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(svg))
}

func GetServiceStatusTimelinePNG(c *gin.Context) {
	data, statusCode, message, err := buildServiceStatusTimelineData(c)
	if message != "" {
		c.String(statusCode, message)
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	pngBytes, err := renderServiceStatusTimelinePNG(data)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Header("Content-Disposition", `inline; filename="service-status.png"`)
	c.Data(http.StatusOK, "image/png", pngBytes)
}
