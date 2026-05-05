package service

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	neturl "net/url"
	"one-api/common"
	"one-api/model"
	"os"
	urlpath "path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const defaultClawBoxPortableRepo = "zwenooo/ClawBox"

var clawBoxPortableCacheLocks sync.Map
var clawBoxPortableGitHubAssetURLCache sync.Map
var trustedClawBoxPortableGitHubHosts = map[string]struct{}{
	"api.github.com":                       {},
	"uploads.github.com":                   {},
	"release-assets.githubusercontent.com": {},
	"objects.githubusercontent.com":        {},
}

var (
	errClawBoxPortableCacheDisabled    = errors.New("portable package server cache is disabled")
	errClawBoxPortableCacheUnknownSize = errors.New("portable package size is unknown")
	errClawBoxPortableCacheNoCapacity  = errors.New("portable package cache has no capacity")
)

type clawBoxGitHubReleaseAsset struct {
	URL                string `json:"url"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
	Size               int64  `json:"size"`
}

type clawBoxGitHubRelease struct {
	ID          int64                       `json:"id"`
	TagName     string                      `json:"tag_name"`
	Name        string                      `json:"name"`
	Body        string                      `json:"body"`
	HTMLURL     string                      `json:"html_url"`
	PublishedAt string                      `json:"published_at"`
	Prerelease  bool                        `json:"prerelease"`
	Assets      []clawBoxGitHubReleaseAsset `json:"assets"`
}

type clawBoxPortableManifestAsset struct {
	Packages []map[string]interface{} `json:"packages"`
}

type ClawBoxPortablePackageDownload struct {
	AssetName      string
	DownloadURL    string
	DownloadSha256 string
	Source         string
	Repo           string
	ReleaseID      int64
	Mode           string
	Platform       string
	Arch           string
	Channel        string
	Kind           string
	BaseVersion    string
	TargetVersion  string
	MinVersion     string
	MaxVersion     string
}

type ClawBoxPortableGitHubSyncParams struct {
	Repo          string
	Version       string
	Platform      string
	Arch          string
	Channel       string
	MinAppVersion string
	SetLatest     bool
}

type ClawBoxPortableGitHubAuthStatus struct {
	Configured bool   `json:"configured"`
	Source     string `json:"source"`
	OptionKey  string `json:"option_key"`
}

func ResolveClawBoxPortableRepo(override string) string {
	repo := strings.TrimSpace(override)
	if repo != "" {
		return repo
	}
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	repo = strings.TrimSpace(common.OptionMap[model.ClawBoxPortableRepoOptionKey])
	if repo == "" {
		return defaultClawBoxPortableRepo
	}
	return repo
}

func ResolveClawBoxPortableChannel(override string) string {
	channel := strings.TrimSpace(override)
	if channel != "" {
		_, _, _, channel = model.NormalizeClawBoxPortableReleaseSelector("portable", "", "", channel)
		return channel
	}
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	channel = strings.TrimSpace(common.OptionMap[model.ClawBoxPortableChannelOptionKey])
	_, _, _, channel = model.NormalizeClawBoxPortableReleaseSelector("portable", "", "", channel)
	return channel
}

func NormalizeClawBoxPortableVersion(raw string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "v"))
}

func NormalizeClawBoxPortableTag(raw string) string {
	version := NormalizeClawBoxPortableVersion(raw)
	if version == "" {
		return ""
	}
	return "v" + version
}

func portableVersionParts(raw string) []int {
	normalized := NormalizeClawBoxPortableVersion(raw)
	if normalized == "" {
		return nil
	}
	parts := strings.Split(normalized, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		value := 0
		for _, ch := range strings.TrimSpace(part) {
			if ch < '0' || ch > '9' {
				value = -1
				break
			}
			value = value*10 + int(ch-'0')
		}
		if value >= 0 {
			out = append(out, value)
		}
	}
	return out
}

func portableVersionCompare(left string, right string) int {
	leftParts := portableVersionParts(left)
	rightParts := portableVersionParts(right)
	count := len(leftParts)
	if len(rightParts) > count {
		count = len(rightParts)
	}
	for i := 0; i < count; i++ {
		leftValue := 0
		rightValue := 0
		if i < len(leftParts) {
			leftValue = leftParts[i]
		}
		if i < len(rightParts) {
			rightValue = rightParts[i]
		}
		if leftValue > rightValue {
			return 1
		}
		if leftValue < rightValue {
			return -1
		}
	}
	return 0
}

func portableVersionGE(left string, right string) bool {
	return portableVersionCompare(left, right) >= 0
}

func portableVersionLE(left string, right string) bool {
	return portableVersionCompare(left, right) <= 0
}

func DefaultClawBoxPortableAssetName(platform string, arch string) string {
	normalizedPlatform := strings.TrimSpace(strings.ToLower(platform))
	if normalizedPlatform == "" {
		normalizedPlatform = "windows"
	}
	normalizedArch := strings.TrimSpace(strings.ToLower(arch))
	if normalizedArch == "" {
		normalizedArch = "x64"
	}
	return fmt.Sprintf("ClawBox-Portable-%s-%s.zip", normalizedPlatform, normalizedArch)
}

func parsePortableSha256(raw string) string {
	line := strings.TrimSpace(raw)
	if line == "" {
		return ""
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.ToLower(fields[0]), "sha256:"))
}

func resolveClawBoxPortableGitHubTokenWithSource() (string, string) {
	common.OptionMapRWMutex.RLock()
	token := strings.TrimSpace(common.OptionMap[model.ClawBoxPortableGitHubTokenKey])
	common.OptionMapRWMutex.RUnlock()
	if token != "" {
		return token, "option"
	}

	token = strings.TrimSpace(os.Getenv("ONEAPI_CLAWBOX_PORTABLE_GITHUB_TOKEN"))
	if token != "" {
		return token, "env:ONEAPI_CLAWBOX_PORTABLE_GITHUB_TOKEN"
	}
	token = strings.TrimSpace(os.Getenv("ONEAPI_CX_COMPAT_GITHUB_TOKEN"))
	if token != "" {
		return token, "env:ONEAPI_CX_COMPAT_GITHUB_TOKEN"
	}
	token = strings.TrimSpace(os.Getenv("ONEAPI_CX_COMPAT_OPENCODE_GITHUB_TOKEN"))
	if token != "" {
		return token, "env:ONEAPI_CX_COMPAT_OPENCODE_GITHUB_TOKEN"
	}
	return "", "none"
}

func ResolveClawBoxPortableGitHubAuthStatus() ClawBoxPortableGitHubAuthStatus {
	token, source := resolveClawBoxPortableGitHubTokenWithSource()
	return ClawBoxPortableGitHubAuthStatus{
		Configured: strings.TrimSpace(token) != "",
		Source:     source,
		OptionKey:  model.ClawBoxPortableGitHubTokenKey,
	}
}

func resolveClawBoxPortableGitHubToken() string {
	token, _ := resolveClawBoxPortableGitHubTokenWithSource()
	return token
}

func applyClawBoxPortableGitHubAuthHeader(req *http.Request) {
	if req == nil || req.URL == nil {
		return
	}
	host := strings.ToLower(strings.TrimSpace(req.URL.Hostname()))
	if _, ok := trustedClawBoxPortableGitHubHosts[host]; !ok {
		return
	}
	token := resolveClawBoxPortableGitHubToken()
	if token == "" {
		return
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		req.Header.Set("Authorization", token)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}

func githubAPIRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ClawBoxPortableSync/1.0")
	applyClawBoxPortableGitHubAuthHeader(req)
	client := &http.Client{Timeout: 20 * time.Second}
	return client.Do(req)
}

func fetchGitHubRelease(repo string, version string) (*clawBoxGitHubRelease, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return nil, errors.New("GitHub repo 不能为空")
	}

	var apiURL string
	if normalizedVersion := NormalizeClawBoxPortableVersion(version); normalizedVersion != "" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/v%s", repo, normalizedVersion)
	} else {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	}

	resp, err := githubAPIRequest(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("GitHub release 查询失败: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var release clawBoxGitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	if strings.TrimSpace(release.TagName) == "" {
		return nil, errors.New("GitHub release 缺少 tag_name")
	}
	return &release, nil
}

func findGitHubReleaseAsset(assets []clawBoxGitHubReleaseAsset, name string) *clawBoxGitHubReleaseAsset {
	target := strings.TrimSpace(name)
	if target == "" {
		return nil
	}
	for i := range assets {
		if strings.EqualFold(strings.TrimSpace(assets[i].Name), target) {
			return &assets[i]
		}
	}
	return nil
}

func fetchSHA256FromURL(url string) (string, error) {
	body, err := fetchClawBoxPortableAssetBytes(url, 2048)
	if err != nil {
		return "", err
	}
	sha := parsePortableSha256(string(body))
	if sha == "" {
		return "", errors.New("sha256 资产内容无效")
	}
	return sha, nil
}

func fetchClawBoxPortableAssetBytes(url string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "ClawBoxPortableSync/1.0")
	applyClawBoxPortableGitHubAuthHeader(req)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("GitHub 资产读取失败: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, err
	}
	return body, nil
}

func fetchClawBoxPortableManifestPackages(url string) ([]map[string]interface{}, error) {
	body, err := fetchClawBoxPortableAssetBytes(url, 1024*1024)
	if err != nil {
		return nil, err
	}

	var manifest clawBoxPortableManifestAsset
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("portable-latest.json 解析失败: %w", err)
	}

	if len(manifest.Packages) == 0 {
		return nil, nil
	}

	packages := make([]map[string]interface{}, 0, len(manifest.Packages))
	for _, item := range manifest.Packages {
		if item == nil {
			continue
		}
		clone := make(map[string]interface{}, len(item))
		for key, value := range item {
			clone[key] = value
		}
		packages = append(packages, clone)
	}
	return packages, nil
}

func stringValue(raw interface{}) string {
	if raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return normalizePortablePlaceholderString(typed)
	case json.Number:
		return normalizePortablePlaceholderString(typed.String())
	default:
		return normalizePortablePlaceholderString(fmt.Sprintf("%v", raw))
	}
}

func normalizePortablePlaceholderString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch strings.ToLower(trimmed) {
	case "", "<nil>", "nil", "<null>", "null", "(null)", "undefined":
		return ""
	default:
		return trimmed
	}
}

func normalizePortablePackageKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "full", "portable-package", "portable_full":
		return "full"
	case "delta", "portable-delta", "portable_delta":
		return "delta"
	default:
		return "full"
	}
}

func portablePackageTargetVersion(item map[string]interface{}, fallbackVersion string) string {
	return firstNonEmpty(
		NormalizeClawBoxPortableVersion(stringValue(item["targetVersion"])),
		NormalizeClawBoxPortableVersion(stringValue(item["version"])),
		NormalizeClawBoxPortableVersion(fallbackVersion),
	)
}

func portablePackageMinCurrentVersion(item map[string]interface{}) string {
	return firstNonEmpty(
		NormalizeClawBoxPortableVersion(stringValue(item["minCurrentVersion"])),
		NormalizeClawBoxPortableVersion(stringValue(item["minVersion"])),
	)
}

func portablePackageMaxCurrentVersion(item map[string]interface{}) string {
	return firstNonEmpty(
		NormalizeClawBoxPortableVersion(stringValue(item["maxCurrentVersion"])),
		NormalizeClawBoxPortableVersion(stringValue(item["maxVersion"])),
	)
}

func portablePackageHasCurrentVersionConstraints(item map[string]interface{}) bool {
	return portablePackageMinCurrentVersion(item) != "" || portablePackageMaxCurrentVersion(item) != ""
}

func portablePackageMatchesCurrentVersionRange(item map[string]interface{}, currentVersion string) bool {
	minVersion := portablePackageMinCurrentVersion(item)
	maxVersion := portablePackageMaxCurrentVersion(item)
	if minVersion == "" && maxVersion == "" {
		return true
	}
	currentVersion = NormalizeClawBoxPortableVersion(currentVersion)
	if currentVersion == "" {
		return false
	}
	if minVersion != "" && !portableVersionGE(currentVersion, minVersion) {
		return false
	}
	if maxVersion != "" && !portableVersionLE(currentVersion, maxVersion) {
		return false
	}
	return true
}

func portablePackageCandidatePreferred(candidate map[string]interface{}, current map[string]interface{}) bool {
	candidateMin := portablePackageMinCurrentVersion(candidate)
	currentMin := portablePackageMinCurrentVersion(current)
	if candidateMin != currentMin {
		if currentMin == "" {
			return candidateMin != ""
		}
		if candidateMin == "" {
			return false
		}
		return portableVersionCompare(candidateMin, currentMin) > 0
	}
	candidateMax := portablePackageMaxCurrentVersion(candidate)
	currentMax := portablePackageMaxCurrentVersion(current)
	if candidateMax != currentMax {
		if currentMax == "" {
			return candidateMax != ""
		}
		if candidateMax == "" {
			return false
		}
		return portableVersionCompare(candidateMax, currentMax) < 0
	}
	candidateTarget := portablePackageTargetVersion(candidate, "")
	currentTarget := portablePackageTargetVersion(current, "")
	if candidateTarget != currentTarget {
		if currentTarget == "" {
			return candidateTarget != ""
		}
		if candidateTarget == "" {
			return false
		}
		return portableVersionCompare(candidateTarget, currentTarget) > 0
	}
	return false
}

func portablePackageAssetNameFromURL(raw string) string {
	parsed, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	base := strings.TrimSpace(urlpath.Base(parsed.Path))
	if base == "" || base == "." || base == "/" {
		return ""
	}
	unescaped, err := neturl.PathUnescape(base)
	if err != nil {
		return base
	}
	return strings.TrimSpace(unescaped)
}

func portablePackageAssetName(item map[string]interface{}) string {
	for _, key := range []string{"assetName", "name"} {
		if value := stringValue(item[key]); value != "" {
			return value
		}
	}
	for _, key := range []string{"assetBrowserUrl", "assetApiUrl", "url"} {
		if value := portablePackageAssetNameFromURL(stringValue(item[key])); value != "" {
			return value
		}
	}
	return ""
}

func int64Value(raw interface{}) int64 {
	switch typed := raw.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		if value, err := typed.Int64(); err == nil {
			return value
		}
	case string:
		if value, err := json.Number(strings.TrimSpace(typed)).Int64(); err == nil {
			return value
		}
	}
	return 0
}

func sourceMetaString(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	return stringValue(meta[key])
}

func resolvePortableGitHubRepo(release *model.ClawBoxPortableRelease) string {
	if release == nil {
		return ""
	}
	if repo := strings.TrimSpace(release.Repo); repo != "" {
		return repo
	}
	return strings.TrimSpace(sourceMetaString(release.SourceMeta(), "repo"))
}

func resolvePortableGitHubReleaseID(release *model.ClawBoxPortableRelease) int64 {
	if release == nil {
		return 0
	}
	return int64Value(release.SourceMeta()["releaseId"])
}

func isGitHubAssetAPIURL(raw string) bool {
	parsed, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host != "api.github.com" && host != "uploads.github.com" {
		return false
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" {
		path = strings.TrimSpace(parsed.Path)
	}
	return strings.Contains(path, "/releases/assets/")
}

func isGitHubReleaseBrowserDownloadURL(raw string) bool {
	parsed, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host != "github.com" {
		return false
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" {
		path = strings.TrimSpace(parsed.Path)
	}
	return strings.Contains(path, "/releases/download/")
}

func resolveGitHubReleaseAssetAPIURL(repo string, releaseID int64, assetName string) (string, error) {
	repo = strings.TrimSpace(repo)
	assetName = strings.TrimSpace(assetName)
	if repo == "" || releaseID <= 0 || assetName == "" {
		return "", errors.New("缺少 GitHub 资产解析参数")
	}

	cacheKey := strings.ToLower(fmt.Sprintf("%s#%d#%s", repo, releaseID, assetName))
	if cached, ok := clawBoxPortableGitHubAssetURLCache.Load(cacheKey); ok {
		if value, valueOK := cached.(string); valueOK && strings.TrimSpace(value) != "" {
			return value, nil
		}
	}

	releaseURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/%d", repo, releaseID)
	resp, err := githubAPIRequest(releaseURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf(
			"GitHub release 资产解析失败: HTTP %d %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	var release clawBoxGitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	asset := findGitHubReleaseAsset(release.Assets, assetName)
	if asset == nil {
		return "", fmt.Errorf("GitHub release 资产不存在: %s", assetName)
	}
	assetURL := strings.TrimSpace(asset.URL)
	if assetURL == "" {
		return "", fmt.Errorf("GitHub release 资产缺少 API URL: %s", assetName)
	}

	clawBoxPortableGitHubAssetURLCache.Store(cacheKey, assetURL)
	return assetURL, nil
}

func buildPortablePackageProxyURL(baseURL string, releaseID int64, assetName string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || releaseID <= 0 {
		return ""
	}
	assetName = normalizePortablePlaceholderString(assetName)
	if assetName == "" {
		return fmt.Sprintf("%s/api/clawbox/update/portable/releases/%d/download", baseURL, releaseID)
	}
	values := neturl.Values{}
	values.Set("asset", assetName)
	return fmt.Sprintf("%s/api/clawbox/update/portable/releases/%d/download?%s", baseURL, releaseID, values.Encode())
}

func portableExtraPackageDownloads(release *model.ClawBoxPortableRelease) []ClawBoxPortablePackageDownload {
	if release == nil {
		return nil
	}

	repo := resolvePortableGitHubRepo(release)
	releaseID := resolvePortableGitHubReleaseID(release)
	rawPackages, ok := release.SourceMeta()["packages"]
	if !ok || rawPackages == nil {
		return nil
	}

	items, ok := rawPackages.([]interface{})
	if !ok {
		return nil
	}

	out := make([]ClawBoxPortablePackageDownload, 0, len(items))
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok || item == nil {
			continue
		}

		kind := normalizePortablePackageKind(stringValue(item["kind"]))
		assetAPIURL := stringValue(item["assetApiUrl"])
		manifestURL := stringValue(item["url"])
		assetBrowserURL := stringValue(item["assetBrowserUrl"])
		url := firstNonEmpty(assetAPIURL, manifestURL, assetBrowserURL)
		sha256 := strings.ToLower(strings.TrimPrefix(stringValue(item["sha256"]), "sha256:"))
		if url == "" || sha256 == "" {
			continue
		}
		assetName := portablePackageAssetName(item)
		if assetName == "" {
			continue
		}
		minVersion := portablePackageMinCurrentVersion(item)
		maxVersion := portablePackageMaxCurrentVersion(item)
		targetVersion := portablePackageTargetVersion(item, release.Version)
		if kind == "full" &&
			strings.EqualFold(assetName, strings.TrimSpace(release.AssetName)) &&
			minVersion == "" &&
			maxVersion == "" &&
			(targetVersion == "" || portableVersionCompare(targetVersion, release.Version) == 0) {
			continue
		}
		source := stringValue(item["source"])
		if source == "" {
			source = strings.TrimSpace(release.Source)
		}
		if strings.EqualFold(source, "github") {
			if isGitHubAssetAPIURL(url) {
				// no-op
			} else if isGitHubReleaseBrowserDownloadURL(url) ||
				isGitHubReleaseBrowserDownloadURL(manifestURL) ||
				isGitHubReleaseBrowserDownloadURL(assetBrowserURL) {
				apiURL, resolveErr := resolveGitHubReleaseAssetAPIURL(repo, releaseID, assetName)
				if resolveErr != nil {
					continue
				}
				url = apiURL
			}
		}

		out = append(out, ClawBoxPortablePackageDownload{
			AssetName:      assetName,
			DownloadURL:    url,
			DownloadSha256: sha256,
			Source:         source,
			Repo:           repo,
			ReleaseID:      releaseID,
			Mode:           firstNonEmpty(stringValue(item["mode"]), release.Mode),
			Platform:       firstNonEmpty(stringValue(item["platform"]), release.Platform),
			Arch:           firstNonEmpty(stringValue(item["arch"]), release.Arch),
			Channel:        firstNonEmpty(stringValue(item["channel"]), release.Channel),
			Kind:           kind,
			BaseVersion:    stringValue(item["baseVersion"]),
			TargetVersion:  targetVersion,
			MinVersion:     minVersion,
			MaxVersion:     maxVersion,
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func ResolveClawBoxPortablePackageDownload(release *model.ClawBoxPortableRelease, assetName string) (*ClawBoxPortablePackageDownload, error) {
	if release == nil {
		return nil, errors.New("portable release 不存在")
	}

	trimmedAssetName := strings.TrimSpace(assetName)
	if normalizePortablePlaceholderString(trimmedAssetName) == "" && trimmedAssetName != "" {
		extraPackages := portableExtraPackageDownloads(release)
		if len(extraPackages) == 1 {
			clone := extraPackages[0]
			return &clone, nil
		}
	}
	trimmedAssetName = normalizePortablePlaceholderString(trimmedAssetName)
	if trimmedAssetName == "" || strings.EqualFold(trimmedAssetName, strings.TrimSpace(release.AssetName)) {
		return &ClawBoxPortablePackageDownload{
			AssetName:      strings.TrimSpace(release.AssetName),
			DownloadURL:    strings.TrimSpace(release.DownloadUrl),
			DownloadSha256: strings.TrimSpace(release.DownloadSha256),
			Source:         strings.TrimSpace(release.Source),
			Repo:           resolvePortableGitHubRepo(release),
			ReleaseID:      resolvePortableGitHubReleaseID(release),
			Mode:           strings.TrimSpace(release.Mode),
			Platform:       strings.TrimSpace(release.Platform),
			Arch:           strings.TrimSpace(release.Arch),
			Channel:        strings.TrimSpace(release.Channel),
			Kind:           "full",
		}, nil
	}

	for _, item := range portableExtraPackageDownloads(release) {
		if strings.EqualFold(trimmedAssetName, item.AssetName) {
			clone := item
			return &clone, nil
		}
	}

	return nil, fmt.Errorf("未找到便携包资产: %s", trimmedAssetName)
}

func extraPortablePackagesFromSourceMeta(release *model.ClawBoxPortableRelease, baseURL string) []map[string]interface{} {
	if release == nil {
		return nil
	}

	out := make([]map[string]interface{}, 0)
	for _, item := range portableExtraPackageDownloads(release) {
		downloadURL := item.DownloadURL
		if proxyURL := buildPortablePackageProxyURL(baseURL, release.Id, item.AssetName); proxyURL != "" {
			downloadURL = proxyURL
		}
		normalized := map[string]interface{}{
			"mode":     item.Mode,
			"kind":     item.Kind,
			"platform": item.Platform,
			"arch":     item.Arch,
			"channel":  item.Channel,
			"url":      downloadURL,
			"sha256":   item.DownloadSha256,
		}
		if item.TargetVersion != "" {
			normalized["targetVersion"] = item.TargetVersion
		}
		if item.MinVersion != "" {
			normalized["minCurrentVersion"] = item.MinVersion
		}
		if item.MaxVersion != "" {
			normalized["maxCurrentVersion"] = item.MaxVersion
		}
		if item.BaseVersion != "" {
			normalized["baseVersion"] = item.BaseVersion
		}
		if item.TargetVersion != "" {
			normalized["targetVersion"] = item.TargetVersion
		}
		out = append(out, normalized)
	}
	return out
}

func parseGitHubPublishedTime(raw string) int64 {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func SyncClawBoxPortableReleaseFromGitHub(params ClawBoxPortableGitHubSyncParams) (*model.ClawBoxPortableRelease, error) {
	repo := ResolveClawBoxPortableRepo(params.Repo)
	channelInput := strings.TrimSpace(params.Channel)
	if channelInput == "" {
		channelInput = ResolveClawBoxPortableChannel("")
	}
	mode, platform, arch, channel := model.NormalizeClawBoxPortableReleaseSelector("portable", params.Platform, params.Arch, channelInput)
	version := NormalizeClawBoxPortableVersion(params.Version)
	if channel != "stable" && version == "" {
		return nil, fmt.Errorf("channel=%s 暂不支持自动同步 GitHub latest，请指定 version", channel)
	}
	minAppVersion := strings.TrimSpace(params.MinAppVersion)
	if minAppVersion == "" {
		minAppVersion = "0.1.0"
	}

	release, err := fetchGitHubRelease(repo, params.Version)
	if err != nil {
		return nil, err
	}

	assetName := DefaultClawBoxPortableAssetName(platform, arch)
	zipAsset := findGitHubReleaseAsset(release.Assets, assetName)
	if zipAsset == nil {
		return nil, fmt.Errorf("GitHub release 缺少整包资产: %s", assetName)
	}
	shaAsset := findGitHubReleaseAsset(release.Assets, assetName+".sha256")
	if shaAsset == nil {
		return nil, fmt.Errorf("GitHub release 缺少校验资产: %s.sha256", assetName)
	}
	manifestAsset := findGitHubReleaseAsset(release.Assets, "portable-latest.json")

	sourceMeta := map[string]interface{}{
		"releaseId":          release.ID,
		"tagName":            release.TagName,
		"releaseName":        release.Name,
		"releaseUrl":         release.HTMLURL,
		"publishedAt":        release.PublishedAt,
		"assetName":          zipAsset.Name,
		"assetSize":          zipAsset.Size,
		"assetApiUrl":        strings.TrimSpace(zipAsset.URL),
		"assetBrowserUrl":    strings.TrimSpace(zipAsset.BrowserDownloadURL),
		"shaAssetName":       shaAsset.Name,
		"shaAssetApiUrl":     strings.TrimSpace(shaAsset.URL),
		"shaAssetBrowserUrl": strings.TrimSpace(shaAsset.BrowserDownloadURL),
	}

	if manifestAsset != nil {
		manifestAssetURL := strings.TrimSpace(manifestAsset.URL)
		if manifestAssetURL == "" {
			manifestAssetURL = strings.TrimSpace(manifestAsset.BrowserDownloadURL)
		}
		manifestPackages, err := fetchClawBoxPortableManifestPackages(manifestAssetURL)
		if err != nil {
			return nil, err
		}
		if len(manifestPackages) > 0 {
			sourceMeta["manifestAssetName"] = manifestAsset.Name
			sourceMeta["manifestAssetApiUrl"] = strings.TrimSpace(manifestAsset.URL)
			sourceMeta["manifestAssetBrowserUrl"] = strings.TrimSpace(manifestAsset.BrowserDownloadURL)
			sourceMeta["packages"] = manifestPackages
		}
	}

	zipDownloadURL := strings.TrimSpace(zipAsset.URL)
	if zipDownloadURL == "" {
		zipDownloadURL = strings.TrimSpace(zipAsset.BrowserDownloadURL)
	}
	shaAssetURL := strings.TrimSpace(shaAsset.URL)
	if shaAssetURL == "" {
		shaAssetURL = strings.TrimSpace(shaAsset.BrowserDownloadURL)
	}
	sha256, err := fetchSHA256FromURL(shaAssetURL)
	if err != nil {
		return nil, err
	}

	return model.UpsertClawBoxPortableRelease(model.ClawBoxPortableReleaseUpsertParams{
		SetLatest:      params.SetLatest,
		Enabled:        true,
		Version:        NormalizeClawBoxPortableVersion(release.TagName),
		Tag:            release.TagName,
		Mode:           mode,
		Platform:       platform,
		Arch:           arch,
		Channel:        channel,
		Source:         "github",
		Repo:           repo,
		AssetName:      zipAsset.Name,
		DownloadUrl:    zipDownloadURL,
		DownloadSha256: sha256,
		ReleasePageUrl: release.HTMLURL,
		ReleaseNotes:   strings.TrimSpace(release.Body),
		MinAppVersion:  minAppVersion,
		PublishedTime:  parseGitHubPublishedTime(release.PublishedAt),
		SourceMeta:     sourceMeta,
	})
}

func selectPreferredPortableManifestPackage(packages []map[string]interface{}, currentVersion string) map[string]interface{} {
	var genericFull map[string]interface{}
	var rangedFull map[string]interface{}
	for _, pkg := range packages {
		if pkg == nil {
			continue
		}
		if normalizePortablePackageKind(stringValue(pkg["kind"])) != "full" {
			continue
		}
		if !portablePackageMatchesCurrentVersionRange(pkg, currentVersion) {
			continue
		}
		if portablePackageHasCurrentVersionConstraints(pkg) {
			if rangedFull == nil || portablePackageCandidatePreferred(pkg, rangedFull) {
				rangedFull = pkg
			}
			continue
		}
		if genericFull == nil {
			genericFull = pkg
		}
	}
	if rangedFull != nil {
		return rangedFull
	}
	return genericFull
}

func BuildClawBoxPortableManifest(baseURL string, mode string, platform string, arch string, channel string, currentVersion string, release *model.ClawBoxPortableRelease) map[string]interface{} {
	mode, platform, arch, channel = model.NormalizeClawBoxPortableReleaseSelector(mode, platform, arch, channel)
	if release == nil {
		return map[string]interface{}{
			"version":        "",
			"mode":           mode,
			"platform":       platform,
			"arch":           arch,
			"channel":        channel,
			"downloadUrl":    "",
			"downloadSha256": "",
			"releaseNotes":   "",
			"minAppVersion":  "",
			"packages":       []map[string]interface{}{},
		}
	}

	mode, platform, arch, channel = model.NormalizeClawBoxPortableReleaseSelector(release.Mode, release.Platform, release.Arch, release.Channel)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	downloadURL := release.DownloadUrl
	if baseURL != "" && release.Id > 0 {
		downloadURL = buildPortablePackageProxyURL(baseURL, release.Id, "")
	}
	packages := append([]map[string]interface{}{
		{
			"mode":          release.Mode,
			"kind":          "full",
			"platform":      release.Platform,
			"arch":          release.Arch,
			"channel":       release.Channel,
			"url":           downloadURL,
			"sha256":        release.DownloadSha256,
			"targetVersion": release.Version,
		},
	}, extraPortablePackagesFromSourceMeta(release, baseURL)...)
	selectedVersion := release.Version
	selectedDownloadURL := downloadURL
	selectedDownloadSha256 := release.DownloadSha256
	if selectedPackage := selectPreferredPortableManifestPackage(packages, currentVersion); selectedPackage != nil {
		selectedVersion = firstNonEmpty(
			NormalizeClawBoxPortableVersion(stringValue(selectedPackage["targetVersion"])),
			NormalizeClawBoxPortableVersion(stringValue(selectedPackage["version"])),
			release.Version,
		)
		selectedDownloadURL = firstNonEmpty(
			stringValue(selectedPackage["downloadUrl"]),
			stringValue(selectedPackage["url"]),
			downloadURL,
		)
		selectedDownloadSha256 = firstNonEmpty(
			stringValue(selectedPackage["downloadSha256"]),
			stringValue(selectedPackage["sha256"]),
			release.DownloadSha256,
		)
	}

	return map[string]interface{}{
		"version":        selectedVersion,
		"mode":           release.Mode,
		"platform":       release.Platform,
		"arch":           release.Arch,
		"channel":        release.Channel,
		"downloadUrl":    selectedDownloadURL,
		"downloadSha256": selectedDownloadSha256,
		"releaseNotes":   release.ReleaseNotes,
		"minAppVersion":  release.MinAppVersion,
		"packages":       packages,
	}
}

func OpenClawBoxPortablePackageDownloadStream(pkg *ClawBoxPortablePackageDownload, rangeHeader string) (*http.Response, error) {
	if pkg == nil {
		return nil, errors.New("portable package 不存在")
	}
	url := strings.TrimSpace(pkg.DownloadURL)
	if url == "" {
		return nil, errors.New("portable package download_url 为空")
	}
	if model.IsClawBoxPortableProxyDownloadURL(url) {
		return nil, errors.New("portable package download_url 错误地指向了 new-api 自己的代理下载地址")
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ClawBoxPortableProxy/1.0")
	req.Header.Set("Accept", "application/octet-stream")
	if trimmedRange := strings.TrimSpace(rangeHeader); trimmedRange != "" {
		req.Header.Set("Range", trimmedRange)
	}
	if strings.EqualFold(strings.TrimSpace(pkg.Source), "github") {
		applyClawBoxPortableGitHubAuthHeader(req)
	}
	client := &http.Client{}
	return client.Do(req)
}

func OpenClawBoxPortableReleaseDownloadStream(release *model.ClawBoxPortableRelease, rangeHeader string) (*http.Response, error) {
	pkg, err := ResolveClawBoxPortablePackageDownload(release, "")
	if err != nil {
		return nil, err
	}
	return OpenClawBoxPortablePackageDownloadStream(pkg, rangeHeader)
}

func clawBoxPortableCacheLock(cachePath string) *sync.Mutex {
	lockKey := strings.TrimSpace(cachePath)
	if lockKey == "" {
		lockKey = "__default__"
	}
	actual, _ := clawBoxPortableCacheLocks.LoadOrStore(lockKey, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func sanitizeClawBoxPortableCacheName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "portable-package.zip"
	}
	reInvalid := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	sanitized := reInvalid.ReplaceAllString(trimmed, "_")
	sanitized = strings.Trim(sanitized, "._-")
	if sanitized == "" {
		return "portable-package.zip"
	}
	return sanitized
}

func clawBoxPortablePersistentCacheEnabled() bool {
	return common.IsDiskCacheEnabled()
}

func IsClawBoxPortableCacheBypassError(err error) bool {
	return errors.Is(err, errClawBoxPortableCacheDisabled) ||
		errors.Is(err, errClawBoxPortableCacheUnknownSize) ||
		errors.Is(err, errClawBoxPortableCacheNoCapacity)
}

func resolveClawBoxPortableCacheRoot() (string, error) {
	if !clawBoxPortablePersistentCacheEnabled() {
		return "", errClawBoxPortableCacheDisabled
	}
	if err := common.EnsureDiskCacheDir(); err != nil {
		return "", err
	}
	return common.GetDiskCacheDir(), nil
}

func resolveClawBoxPortableCachePath(release *model.ClawBoxPortableRelease) (string, error) {
	if release == nil {
		return "", errors.New("portable release 不存在")
	}
	root, err := resolveClawBoxPortableCacheRoot()
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(release.Version)
	if version == "" {
		version = "unknown"
	}
	sha := strings.TrimSpace(strings.ToLower(release.DownloadSha256))
	if sha == "" {
		sha = "nohash"
	}
	fileName := fmt.Sprintf("clawbox-portable-release-%d-%s-%s-%s", release.Id, version, sha, sanitizeClawBoxPortableCacheName(release.AssetName))
	return filepath.Join(root, fileName), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func clawBoxPortableExpectedSize(release *model.ClawBoxPortableRelease) int64 {
	if release == nil {
		return 0
	}
	meta := release.SourceMeta()
	for _, key := range []string{"assetSize", "size"} {
		value, ok := meta[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case int64:
			if typed > 0 {
				return typed
			}
		case int:
			if typed > 0 {
				return int64(typed)
			}
		case float64:
			if typed > 0 {
				return int64(typed)
			}
		case json.Number:
			if parsed, err := typed.Int64(); err == nil && parsed > 0 {
				return parsed
			}
		case string:
			if parsed, err := json.Number(strings.TrimSpace(typed)).Int64(); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return 0
}

func reserveClawBoxPortableCacheBytes(release *model.ClawBoxPortableRelease) (int64, error) {
	size := clawBoxPortableExpectedSize(release)
	if size <= 0 {
		return 0, errClawBoxPortableCacheUnknownSize
	}
	if common.TryReserveDiskCache(size) {
		return size, nil
	}
	_ = common.CleanupOldDiskCacheFiles(24 * time.Hour)
	common.SyncDiskCacheStats()
	if common.TryReserveDiskCache(size) {
		return size, nil
	}
	return 0, errClawBoxPortableCacheNoCapacity
}

func downloadClawBoxPortableReleaseToFile(release *model.ClawBoxPortableRelease, filePath string) (int64, error) {
	resp, err := OpenClawBoxPortableReleaseDownloadStream(release, "")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, fmt.Errorf("上游下载失败: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	parentDir := filepath.Dir(filePath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return 0, err
	}

	tempFile, err := os.CreateTemp(parentDir, ".portable-cache-*")
	if err != nil {
		return 0, err
	}
	tempPath := tempFile.Name()
	shouldCleanup := true
	defer func() {
		_ = tempFile.Close()
		if shouldCleanup {
			_ = os.Remove(tempPath)
		}
	}()

	hasher := sha256.New()
	writer := io.MultiWriter(tempFile, hasher)
	written, err := io.Copy(writer, resp.Body)
	if err != nil {
		return 0, err
	}
	if written <= 0 {
		return 0, errors.New("上游下载为空")
	}
	if err := tempFile.Sync(); err != nil {
		return 0, err
	}
	if err := tempFile.Close(); err != nil {
		return 0, err
	}

	expected := strings.TrimSpace(strings.ToLower(release.DownloadSha256))
	actual := fmt.Sprintf("%x", hasher.Sum(nil))
	if expected != "" && actual != expected {
		return 0, fmt.Errorf("下载文件 SHA256 校验失败: expected=%s actual=%s", expected, actual)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		return 0, err
	}
	shouldCleanup = false
	return written, nil
}

func EnsureClawBoxPortableReleaseCached(release *model.ClawBoxPortableRelease) (string, fs.FileInfo, error) {
	cachePath, err := resolveClawBoxPortableCachePath(release)
	if err != nil {
		return "", nil, err
	}

	if info, statErr := os.Stat(cachePath); statErr == nil && !info.IsDir() {
		common.IncrementDiskCacheHits()
		return cachePath, info, nil
	}

	lock := clawBoxPortableCacheLock(cachePath)
	lock.Lock()
	defer lock.Unlock()

	if info, statErr := os.Stat(cachePath); statErr == nil && !info.IsDir() {
		common.IncrementDiskCacheHits()
		return cachePath, info, nil
	}

	reservedBytes, err := reserveClawBoxPortableCacheBytes(release)
	if err != nil {
		return "", nil, err
	}
	committed := false
	defer func() {
		if !committed {
			common.ReleaseDiskCacheReservation(reservedBytes)
		}
	}()

	actualSize, err := downloadClawBoxPortableReleaseToFile(release, cachePath)
	if err != nil {
		return "", nil, err
	}
	common.CommitDiskCacheReservation(reservedBytes, actualSize)
	committed = true

	info, err := os.Stat(cachePath)
	if err != nil {
		return "", nil, err
	}
	return cachePath, info, nil
}

func OpenClawBoxPortableCachedFile(release *model.ClawBoxPortableRelease) (*os.File, fs.FileInfo, error) {
	cachePath, info, err := EnsureClawBoxPortableReleaseCached(release)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(cachePath)
	if err != nil {
		return nil, nil, err
	}
	return file, info, nil
}

func ClawBoxPortableContentType(assetName string) string {
	contentType := strings.TrimSpace(mime.TypeByExtension(strings.ToLower(filepath.Ext(assetName))))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}
