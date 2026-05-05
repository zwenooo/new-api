package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"one-api/common"
	"one-api/model"
	"one-api/service"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type clawBoxActivationCheckRequest struct {
	ActivationCode string `json:"activation_code"`
}

type clawBoxRegisterRequest struct {
	ActivationCode string `json:"activation_code"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	Remark         string `json:"remark"`
}

func GetClawBoxBootstrap(c *gin.Context) {
	bootstrap := model.ClawBoxBootstrap()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    bootstrap,
	})
}

func CheckClawBoxActivationCode(c *gin.Context) {
	var req clawBoxActivationCheckRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	if err := model.CheckClawBoxActivationCode(req.ActivationCode); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"valid": true,
		},
	})
}

func RegisterClawBox(c *gin.Context) {
	var req clawBoxRegisterRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	user, _, err := model.RegisterClawBoxUserWithActivation(model.ClawBoxRegisterParams{
		ActivationCode: req.ActivationCode,
		Username:       req.Username,
		Password:       req.Password,
		Remark:         model.ClawBoxUserRemark,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := user.FillUserById(); err != nil {
		common.ApiError(c, err)
		return
	}

	setupLogin(user, c)
}

func respondClawBoxRelayToken(c *gin.Context, userID int) {
	token, err := model.EnsureClawBoxRelayToken(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	allowedGroupIDs, productID, err := model.ResolveClawBoxRelayGroupIDsTx(nil, userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	managedConfigPatch, err := model.ClawBoxManagedOpenClawConfigPatch()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"valid":                true,
			"token":                token.Key,
			"token_name":           token.Name,
			"product_mode":         model.ClawBoxProductModeEnabled(),
			"product_id":           productID,
			"allowed_group_ids":    allowedGroupIDs,
			"managed_config_patch": managedConfigPatch,
		},
	})
}

func GetClawBoxRelayToken(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录",
		})
		return
	}

	if c.Request.Method == http.MethodGet {
		// Backward compatibility: older shipped ClawBox builds still request the relay token
		// via GET without portable-medium/device payloads.
		respondClawBoxRelayToken(c, userID)
		return
	}

	var req struct {
		DeviceID       string                            `json:"device_id"`
		Platform       string                            `json:"platform"`
		PortableMedium *model.ClawBoxPortableMediumInput `json:"portable_medium"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	decision, err := model.VerifyClawBoxAccess(
		userID,
		strings.TrimSpace(req.DeviceID),
		req.Platform,
		req.PortableMedium,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !decision.Allowed {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"valid":      false,
				"reason":     decision.Reason,
				"error_code": decision.ErrorCode,
			},
		})
		return
	}

	respondClawBoxRelayToken(c, userID)
}

// ========== 更新清单 ==========

const (
	clawBoxBundledUpdateKey = "ClawBoxBundledUpdate"
)

type clawBoxPortableGitHubSyncRequest struct {
	Version       string `json:"version"`
	Repo          string `json:"repo"`
	Platform      string `json:"platform"`
	Arch          string `json:"arch"`
	Channel       string `json:"channel"`
	MinAppVersion string `json:"min_app_version"`
	SetLatest     *bool  `json:"set_latest,omitempty"`
}

type clawBoxPortableReleaseRequest struct {
	Version        string `json:"version"`
	Tag            string `json:"tag"`
	Mode           string `json:"mode"`
	Platform       string `json:"platform"`
	Arch           string `json:"arch"`
	Channel        string `json:"channel"`
	Source         string `json:"source"`
	Repo           string `json:"repo"`
	AssetName      string `json:"asset_name"`
	DownloadUrl    string `json:"download_url"`
	DownloadSha256 string `json:"download_sha256"`
	ReleasePageUrl string `json:"release_page_url"`
	ReleaseNotes   string `json:"release_notes"`
	MinAppVersion  string `json:"min_app_version"`
	SetLatest      *bool  `json:"set_latest,omitempty"`
}

type clawBoxPortableGitHubTokenRequest struct {
	Token string `json:"token"`
}

type clawBoxBundledUpdateNodeInfo struct {
	Version string `json:"version"`
	Url     string `json:"url"`
	Sha256  string `json:"sha256"`
}

type clawBoxBundledUpdatePackageInfo struct {
	Url    string `json:"url"`
	Sha256 string `json:"sha256"`
}

type clawBoxBundledUpdateDownloads struct {
	Windows *clawBoxBundledUpdatePackageInfo `json:"windows,omitempty"`
	MacOS   *clawBoxBundledUpdatePackageInfo `json:"macos,omitempty"`
}

type clawBoxBundledUpdateRequest struct {
	Version       string                         `json:"version"`
	BundledUrl    string                         `json:"bundledUrl"`
	BundledSha256 string                         `json:"bundledSha256"`
	Downloads     *clawBoxBundledUpdateDownloads `json:"downloads,omitempty"`
	Node          *clawBoxBundledUpdateNodeInfo  `json:"node,omitempty"`
	ReleaseNotes  string                         `json:"releaseNotes"`
	MinAppVersion string                         `json:"minAppVersion"`
}

func respondClawBoxEmptyManifest(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": "",
	})
}

func getClawBoxUpdateManifest(c *gin.Context, optionKey string) {
	value, exists := common.OptionMap[optionKey]
	if !exists || strings.TrimSpace(value) == "" {
		respondClawBoxEmptyManifest(c)
		return
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal([]byte(value), &manifest); err != nil {
		respondClawBoxEmptyManifest(c)
		return
	}

	c.JSON(http.StatusOK, manifest)
}

// GetClawBoxBundledUpdate 公开接口，客户端检查 bundled 更新时调用
func GetClawBoxBundledUpdate(c *gin.Context) {
	getClawBoxUpdateManifest(c, clawBoxBundledUpdateKey)
}

// SetClawBoxBundledUpdate 管理员接口，设置 bundled 更新清单
func SetClawBoxBundledUpdate(c *gin.Context) {
	var req clawBoxBundledUpdateRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	manifest := map[string]interface{}{
		"version":       strings.TrimSpace(req.Version),
		"bundledUrl":    strings.TrimSpace(req.BundledUrl),
		"bundledSha256": strings.TrimSpace(req.BundledSha256),
		"releaseNotes":  strings.TrimSpace(req.ReleaseNotes),
		"minAppVersion": strings.TrimSpace(req.MinAppVersion),
	}

	if req.Downloads != nil {
		downloads := map[string]map[string]string{}
		if req.Downloads.Windows != nil {
			windowsUrl := strings.TrimSpace(req.Downloads.Windows.Url)
			windowsSha256 := strings.TrimSpace(req.Downloads.Windows.Sha256)
			if windowsUrl != "" || windowsSha256 != "" {
				downloads["windows"] = map[string]string{
					"url":    windowsUrl,
					"sha256": windowsSha256,
				}
			}
		}
		if req.Downloads.MacOS != nil {
			macOSUrl := strings.TrimSpace(req.Downloads.MacOS.Url)
			macOSSha256 := strings.TrimSpace(req.Downloads.MacOS.Sha256)
			if macOSUrl != "" || macOSSha256 != "" {
				downloads["macos"] = map[string]string{
					"url":    macOSUrl,
					"sha256": macOSSha256,
				}
			}
		}
		if len(downloads) > 0 {
			manifest["downloads"] = downloads
		}
	}

	if req.Node != nil {
		nodeVersion := strings.TrimSpace(req.Node.Version)
		nodeUrl := strings.TrimSpace(req.Node.Url)
		nodeSha256 := strings.TrimSpace(req.Node.Sha256)
		if nodeVersion != "" && nodeUrl != "" && nodeSha256 != "" {
			manifest["node"] = map[string]string{
				"version": nodeVersion,
				"url":     nodeUrl,
				"sha256":  nodeSha256,
			}
		}
	}

	version := strings.TrimSpace(req.Version)
	if version == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "version 不能为空",
		})
		return
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "序列化失败",
		})
		return
	}

	if err := model.UpdateOption(clawBoxBundledUpdateKey, string(data)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "保存失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    manifest,
	})
}

func clawBoxPortableResponseBaseURL(c *gin.Context) string {
	firstForwardedValue := func(raw string) string {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return ""
		}
		parts := strings.Split(raw, ",")
		if len(parts) == 0 {
			return ""
		}
		return strings.TrimSpace(parts[0])
	}

	scheme := strings.ToLower(firstForwardedValue(c.GetHeader("X-Forwarded-Proto")))
	switch scheme {
	case "http", "https":
	default:
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := firstForwardedValue(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func clawBoxPortableSelectorFromQuery(c *gin.Context) (string, string, string, string) {
	return model.NormalizeClawBoxPortableReleaseSelector("portable", c.Query("platform"), c.Query("arch"), c.Query("channel"))
}

func clawBoxPortableUpdateStatusPayload(mode string, platform string, arch string, channel string) gin.H {
	enabled := model.ClawBoxPortableUpdateEnabled()
	message := ""
	if !enabled {
		message = "管理员已关闭 ClawBox Portable 更新"
	}
	return gin.H{
		"enabled":       enabled,
		"updateEnabled": enabled,
		"message":       message,
		"mode":          mode,
		"platform":      platform,
		"arch":          arch,
		"channel":       channel,
	}
}

func clawBoxPortableDisabledManifest(mode string, platform string, arch string, channel string, currentVersion string) map[string]interface{} {
	manifest := service.BuildClawBoxPortableManifest("", mode, platform, arch, channel, currentVersion, nil)
	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion != "" {
		manifest["version"] = currentVersion
		manifest["selectedVersion"] = currentVersion
	}
	manifest["currentVersion"] = currentVersion
	for key, value := range clawBoxPortableUpdateStatusPayload(mode, platform, arch, channel) {
		manifest[key] = value
	}
	return manifest
}

func respondClawBoxPortableUpdateDisabled(c *gin.Context, mode string, platform string, arch string, channel string) {
	response := clawBoxPortableUpdateStatusPayload(mode, platform, arch, channel)
	response["success"] = false
	response["error_code"] = "portable_update_disabled"
	c.JSON(http.StatusForbidden, response)
}

func clawBoxPortableReleaseManifestURL(baseURL string, releaseID int64) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || releaseID <= 0 {
		return ""
	}
	return baseURL + "/api/clawbox/update/portable/releases/" + strconv.FormatInt(releaseID, 10) + "/manifest"
}

func loadPublicClawBoxPortableRelease(c *gin.Context) (*model.ClawBoxPortableRelease, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的版本 ID"})
		return nil, false
	}

	release, err := model.GetClawBoxPortableReleaseByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "版本不存在"})
		return nil, false
	}

	mode, platform, arch, channel := model.NormalizeClawBoxPortableReleaseSelector(release.Mode, release.Platform, release.Arch, release.Channel)
	if !model.ClawBoxPortableUpdateEnabled() {
		respondClawBoxPortableUpdateDisabled(c, mode, platform, arch, channel)
		return nil, false
	}
	if !release.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "版本不存在"})
		return nil, false
	}
	return release, true
}

func respondClawBoxPortableManifestError(c *gin.Context, mode string, platform string, arch string, channel string, err error) {
	response := gin.H{
		"success":    false,
		"version":    "",
		"mode":       mode,
		"platform":   platform,
		"arch":       arch,
		"channel":    channel,
		"message":    "portable latest manifest 暂时不可用",
		"error_code": "portable_manifest_unavailable",
	}
	if err != nil {
		response["details"] = err.Error()
	}
	c.JSON(http.StatusServiceUnavailable, response)
}

func GetClawBoxPortableUpdateStatus(c *gin.Context) {
	mode, platform, arch, channel := clawBoxPortableSelectorFromQuery(c)
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, clawBoxPortableUpdateStatusPayload(mode, platform, arch, channel))
}

func GetClawBoxInstalledUpdate(c *gin.Context) {
	target := c.Query("target")
	arch := c.Query("arch")
	currentVersion := strings.TrimSpace(c.Query("current_version"))
	update, err := service.ResolveClawBoxInstalledUpdate(clawBoxPortableResponseBaseURL(c), target, arch, currentVersion)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"message": err.Error(),
		})
		return
	}
	if update == nil {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, update)
}

func DownloadClawBoxInstalledRelease(c *gin.Context) {
	version := strings.TrimSpace(c.Param("version"))
	if version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的安装版版本号"})
		return
	}

	selectedAssetName := strings.TrimSpace(c.Query("asset"))
	pkg, err := service.ResolveClawBoxInstalledPackageDownload(version, selectedAssetName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": err.Error()})
		return
	}

	upstreamResp, upstreamErr := service.OpenClawBoxInstalledPackageDownloadStream(pkg, c.GetHeader("Range"))
	if upstreamErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": upstreamErr.Error()})
		return
	}
	defer upstreamResp.Body.Close()

	if upstreamResp.StatusCode != http.StatusOK && upstreamResp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(io.LimitReader(upstreamResp.Body, 1024))
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "上游下载失败: HTTP " + strconv.Itoa(upstreamResp.StatusCode) + " " + strings.TrimSpace(string(body)),
		})
		return
	}

	for _, headerKey := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag", "Cache-Control"} {
		if value := strings.TrimSpace(upstreamResp.Header.Get(headerKey)); value != "" {
			c.Header(headerKey, value)
		}
	}
	c.Header("Content-Type", service.ClawBoxInstalledContentType(pkg.AssetName))
	c.Header("Content-Disposition", "attachment; filename=\""+pkg.AssetName+"\"")
	c.Status(upstreamResp.StatusCode)
	if _, copyErr := io.Copy(c.Writer, upstreamResp.Body); copyErr != nil && c.Request.Context().Err() == nil {
		common.SysError("proxy clawbox installed upstream download failed: " + copyErr.Error())
	}
}

func GetClawBoxPortableUpdate(c *gin.Context) {
	mode, platform, arch, channel := clawBoxPortableSelectorFromQuery(c)
	currentVersion := strings.TrimSpace(c.Query("current_version"))
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")

	if !model.ClawBoxPortableUpdateEnabled() {
		c.JSON(http.StatusOK, clawBoxPortableDisabledManifest(mode, platform, arch, channel, currentVersion))
		return
	}

	release, err := model.GetLatestClawBoxPortableRelease(mode, platform, arch, channel)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manifest := service.BuildClawBoxPortableManifest("", mode, platform, arch, channel, currentVersion, nil)
			for key, value := range clawBoxPortableUpdateStatusPayload(mode, platform, arch, channel) {
				manifest[key] = value
			}
			c.JSON(http.StatusOK, manifest)
			return
		}
		common.SysError("get clawbox portable latest failed: " + err.Error())
		respondClawBoxPortableManifestError(c, mode, platform, arch, channel, err)
		return
	}

	manifest := service.BuildClawBoxPortableManifest(clawBoxPortableResponseBaseURL(c), mode, platform, arch, channel, currentVersion, release)
	for key, value := range clawBoxPortableUpdateStatusPayload(mode, platform, arch, channel) {
		manifest[key] = value
	}
	c.JSON(http.StatusOK, manifest)
}

func buildClawBoxPortableReleaseItems(baseURL string, releases []*model.ClawBoxPortableRelease, onlyEnabled bool) []gin.H {
	items := make([]gin.H, 0, len(releases))
	for _, release := range releases {
		if release == nil {
			continue
		}
		if onlyEnabled && !release.Enabled {
			continue
		}
		downloadURL := release.DownloadUrl
		if strings.TrimSpace(baseURL) != "" {
			downloadURL = strings.TrimRight(baseURL, "/") + "/api/clawbox/update/portable/releases/" + strconv.FormatInt(release.Id, 10) + "/download"
		}
		items = append(items, gin.H{
			"id":                 release.Id,
			"enabled":            release.Enabled,
			"is_latest":          release.IsLatest,
			"version":            release.Version,
			"tag":                release.Tag,
			"mode":               release.Mode,
			"platform":           release.Platform,
			"arch":               release.Arch,
			"channel":            release.Channel,
			"source":             release.Source,
			"repo":               release.Repo,
			"asset_name":         release.AssetName,
			"download_url":       release.DownloadUrl,
			"proxy_download_url": downloadURL,
			"download_sha256":    release.DownloadSha256,
			"release_page_url":   release.ReleasePageUrl,
			"release_notes":      release.ReleaseNotes,
			"min_app_version":    release.MinAppVersion,
			"published_time":     release.PublishedTime,
			"created_time":       release.CreatedTime,
			"updated_time":       release.UpdatedTime,
		})
	}
	return items
}

func buildClawBoxPortablePublicReleaseCatalogItems(baseURL string, releases []*model.ClawBoxPortableRelease) []gin.H {
	items := make([]gin.H, 0, len(releases))
	for _, release := range releases {
		if release == nil || !release.Enabled {
			continue
		}
		items = append(items, gin.H{
			"id":              release.Id,
			"is_latest":       release.IsLatest,
			"version":         release.Version,
			"mode":            release.Mode,
			"platform":        release.Platform,
			"arch":            release.Arch,
			"channel":         release.Channel,
			"manifest_url":    clawBoxPortableReleaseManifestURL(baseURL, release.Id),
			"release_notes":   release.ReleaseNotes,
			"min_app_version": release.MinAppVersion,
			"published_time":  release.PublishedTime,
		})
	}
	return items
}

func GetClawBoxPortableReleaseCatalog(c *gin.Context) {
	mode, platform, arch, channel := clawBoxPortableSelectorFromQuery(c)
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")

	if !model.ClawBoxPortableUpdateEnabled() {
		respondClawBoxPortableUpdateDisabled(c, mode, platform, arch, channel)
		return
	}

	releases, err := model.ListClawBoxPortableReleases(mode, platform, arch, channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, buildClawBoxPortablePublicReleaseCatalogItems(clawBoxPortableResponseBaseURL(c), releases))
}

func GetClawBoxPortableReleaseManifest(c *gin.Context) {
	release, ok := loadPublicClawBoxPortableRelease(c)
	if !ok {
		return
	}

	currentVersion := strings.TrimSpace(c.Query("current_version"))
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")

	manifest := service.BuildClawBoxPortableManifest(
		clawBoxPortableResponseBaseURL(c),
		release.Mode,
		release.Platform,
		release.Arch,
		release.Channel,
		currentVersion,
		release,
	)
	for key, value := range clawBoxPortableUpdateStatusPayload(release.Mode, release.Platform, release.Arch, release.Channel) {
		manifest[key] = value
	}
	c.JSON(http.StatusOK, manifest)
}

func ListClawBoxPortableReleases(c *gin.Context) {
	mode, platform, arch, channel := clawBoxPortableSelectorFromQuery(c)

	releases, err := model.ListClawBoxPortableReleases(mode, platform, arch, channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, buildClawBoxPortableReleaseItems(clawBoxPortableResponseBaseURL(c), releases, false))
}

func GetClawBoxPortableGitHubToken(c *gin.Context) {
	common.ApiSuccess(c, service.ResolveClawBoxPortableGitHubAuthStatus())
}

func SetClawBoxPortableGitHubToken(c *gin.Context) {
	var req clawBoxPortableGitHubTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	token := strings.TrimSpace(req.Token)
	if token == "" {
		common.ApiErrorMsg(c, "GitHub Token 不能为空，如需移除请点击清空")
		return
	}

	if err := model.UpdateOption(model.ClawBoxPortableGitHubTokenKey, token); err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, service.ResolveClawBoxPortableGitHubAuthStatus())
}

func ClearClawBoxPortableGitHubToken(c *gin.Context) {
	if err := model.UpdateOption(model.ClawBoxPortableGitHubTokenKey, ""); err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, service.ResolveClawBoxPortableGitHubAuthStatus())
}

func SyncClawBoxPortableReleaseFromGitHub(c *gin.Context) {
	var req clawBoxPortableGitHubSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	setLatest := true
	if req.SetLatest != nil {
		setLatest = *req.SetLatest
	}

	release, err := service.SyncClawBoxPortableReleaseFromGitHub(service.ClawBoxPortableGitHubSyncParams{
		Repo:          req.Repo,
		Version:       req.Version,
		Platform:      req.Platform,
		Arch:          req.Arch,
		Channel:       req.Channel,
		MinAppVersion: req.MinAppVersion,
		SetLatest:     setLatest,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"id":        release.Id,
		"version":   release.Version,
		"tag":       release.Tag,
		"is_latest": release.IsLatest,
	})
}

func CreateClawBoxPortableRelease(c *gin.Context) {
	var req clawBoxPortableReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	setLatest := true
	if req.SetLatest != nil {
		setLatest = *req.SetLatest
	}

	release, err := model.UpsertClawBoxPortableRelease(model.ClawBoxPortableReleaseUpsertParams{
		SetLatest:      setLatest,
		Enabled:        true,
		Version:        req.Version,
		Tag:            req.Tag,
		Mode:           req.Mode,
		Platform:       req.Platform,
		Arch:           req.Arch,
		Channel:        req.Channel,
		Source:         req.Source,
		Repo:           req.Repo,
		AssetName:      req.AssetName,
		DownloadUrl:    req.DownloadUrl,
		DownloadSha256: req.DownloadSha256,
		ReleasePageUrl: req.ReleasePageUrl,
		ReleaseNotes:   req.ReleaseNotes,
		MinAppVersion:  req.MinAppVersion,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, release)
}

func ActivateClawBoxPortableRelease(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的版本 ID")
		return
	}
	release, err := model.ActivateClawBoxPortableRelease(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"id":      release.Id,
		"version": release.Version,
	})
}

func DeleteClawBoxPortableRelease(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的版本 ID")
		return
	}

	if err := model.DeleteClawBoxPortableRelease(id); err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"id": id,
	})
}

func DownloadClawBoxPortableRelease(c *gin.Context) {
	release, ok := loadPublicClawBoxPortableRelease(c)
	if !ok {
		return
	}

	selectedAssetName := strings.TrimSpace(c.Query("asset"))
	selectedPackage, err := service.ResolveClawBoxPortablePackageDownload(release, selectedAssetName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": err.Error()})
		return
	}

	serveUpstream := func() {
		upstreamResp, upstreamErr := service.OpenClawBoxPortablePackageDownloadStream(selectedPackage, c.GetHeader("Range"))
		if upstreamErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": upstreamErr.Error()})
			return
		}
		defer upstreamResp.Body.Close()

		if upstreamResp.StatusCode != http.StatusOK && upstreamResp.StatusCode != http.StatusPartialContent {
			body, _ := io.ReadAll(io.LimitReader(upstreamResp.Body, 1024))
			c.JSON(http.StatusBadGateway, gin.H{
				"success": false,
				"message": "上游下载失败: HTTP " + strconv.Itoa(upstreamResp.StatusCode) + " " + strings.TrimSpace(string(body)),
			})
			return
		}

		for _, headerKey := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag", "Cache-Control"} {
			if value := strings.TrimSpace(upstreamResp.Header.Get(headerKey)); value != "" {
				c.Header(headerKey, value)
			}
		}
		c.Header("Content-Type", service.ClawBoxPortableContentType(selectedPackage.AssetName))
		c.Header("Content-Disposition", "attachment; filename=\""+selectedPackage.AssetName+"\"")
		c.Status(upstreamResp.StatusCode)
		if _, copyErr := io.Copy(c.Writer, upstreamResp.Body); copyErr != nil && c.Request.Context().Err() == nil {
			common.SysError("proxy clawbox portable upstream download failed: " + copyErr.Error())
		}
	}

	if !strings.EqualFold(strings.TrimSpace(selectedPackage.AssetName), strings.TrimSpace(release.AssetName)) {
		serveUpstream()
		return
	}

	file, info, err := service.OpenClawBoxPortableCachedFile(release)
	if err != nil {
		if service.IsClawBoxPortableCacheBypassError(err) {
			serveUpstream()
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	defer file.Close()

	c.Header("Content-Type", service.ClawBoxPortableContentType(release.AssetName))
	c.Header("Content-Disposition", "attachment; filename=\""+release.AssetName+"\"")
	c.Header("Accept-Ranges", "bytes")
	http.ServeContent(c.Writer, c.Request, release.AssetName, info.ModTime(), file)
	if err := c.Request.Context().Err(); err != nil {
		common.SysError("serve clawbox portable cached download interrupted: " + err.Error())
	}
}

// UnregisterClawBoxDevice 退出登录时解绑设备席位
func UnregisterClawBoxDevice(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "未登录"})
		return
	}

	var req struct {
		DeviceID string `json:"device_id"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}

	if err := model.RemoveClawBoxDevice(userID, strings.TrimSpace(req.DeviceID)); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": nil})
}
func VerifyClawBoxAuth(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "未登录"})
		return
	}

	var req struct {
		DeviceID       string                            `json:"device_id"`
		Platform       string                            `json:"platform"`
		PortableMedium *model.ClawBoxPortableMediumInput `json:"portable_medium"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}

	decision, err := model.VerifyClawBoxAccess(
		userID,
		strings.TrimSpace(req.DeviceID),
		req.Platform,
		req.PortableMedium,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"valid":      decision.Allowed,
			"reason":     decision.Reason,
			"error_code": decision.ErrorCode,
		},
	})
}

func ReactivateClawBoxPortableMedium(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "未登录"})
		return
	}

	var req struct {
		ActivationCode string                            `json:"activation_code"`
		DeviceID       string                            `json:"device_id"`
		PortableMedium *model.ClawBoxPortableMediumInput `json:"portable_medium"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}

	decision, err := model.ReactivateClawBoxPortableMedium(
		userID,
		req.ActivationCode,
		req.DeviceID,
		req.PortableMedium,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !decision.Allowed {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"valid":      false,
				"reason":     decision.Reason,
				"error_code": decision.ErrorCode,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"valid": true,
		},
	})
}
