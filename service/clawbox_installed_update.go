package service

import (
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultClawBoxInstalledRepo     = "zwenooo/ClawBox"
	clawBoxInstalledRepoEnvKey      = "ONEAPI_CLAWBOX_INSTALLED_REPO"
	clawBoxInstalledReleaseCacheTTL = 30 * time.Second
)

var (
	clawBoxInstalledReleaseCache = struct {
		mu    sync.RWMutex
		items map[string]clawBoxInstalledReleaseCacheEntry
	}{}
	clawBoxInstalledSignatureCache sync.Map // map[string]string
)

type clawBoxInstalledReleaseCacheEntry struct {
	release   *clawBoxGitHubRelease
	fetchedAt time.Time
}

type ClawBoxInstalledUpdateResponse struct {
	Version   string `json:"version"`
	PubDate   string `json:"pub_date"`
	URL       string `json:"url"`
	Signature string `json:"signature"`
	Notes     string `json:"notes"`
}

type ClawBoxInstalledPackageDownload struct {
	AssetName   string
	DownloadURL string
	Source      string
	Repo        string
	Version     string
}

// ResolveClawBoxInstalledUpdate returns the Tauri updater payload for Windows x64 when an update exists.
func ResolveClawBoxInstalledUpdate(baseURL string, target string, arch string, currentVersion string) (*ClawBoxInstalledUpdateResponse, error) {
	if !isClawBoxInstalledTarget(target, arch) {
		return nil, nil
	}
	repo := resolveClawBoxInstalledRepo()
	release, err := cachedClawBoxInstalledRelease(repo)
	if err != nil {
		return nil, err
	}
	currentVersion = NormalizeClawBoxInstalledSemver(currentVersion)
	resp, available, err := buildClawBoxInstalledUpdateResponse(baseURL, repo, release, currentVersion)
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, nil
	}
	return resp, nil
}

func isClawBoxInstalledTarget(target string, arch string) bool {
	target = strings.TrimSpace(strings.ToLower(target))
	arch = strings.TrimSpace(strings.ToLower(arch))
	if target != "" && !strings.Contains(target, "windows") {
		return false
	}
	if arch != "" && arch != "x64" && arch != "amd64" && arch != "x86_64" {
		return false
	}
	return true
}

func resolveClawBoxInstalledRepo() string {
	repo := strings.TrimSpace(os.Getenv(clawBoxInstalledRepoEnvKey))
	if repo != "" {
		return repo
	}
	return defaultClawBoxInstalledRepo
}

func normalizeClawBoxInstalledVersionInput(raw string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "v"))
}

func parseClawBoxInstalledVersionPart(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func NormalizeClawBoxInstalledSemver(raw string) string {
	normalized := normalizeClawBoxInstalledVersionInput(raw)
	if normalized == "" {
		return ""
	}

	parts := strings.Split(normalized, ".")
	switch len(parts) {
	case 3:
		major, okMajor := parseClawBoxInstalledVersionPart(parts[0])
		minor, okMinor := parseClawBoxInstalledVersionPart(parts[1])
		patch, okPatch := parseClawBoxInstalledVersionPart(parts[2])
		if !okMajor || !okMinor || !okPatch {
			return ""
		}
		return fmt.Sprintf("%d.%d.%d", major, minor, patch)
	case 4:
		year, okYear := parseClawBoxInstalledVersionPart(parts[0])
		month, okMonth := parseClawBoxInstalledVersionPart(parts[1])
		day, okDay := parseClawBoxInstalledVersionPart(parts[2])
		build, okBuild := parseClawBoxInstalledVersionPart(parts[3])
		if !okYear || !okMonth || !okDay || !okBuild {
			return ""
		}
		if month < 1 || month > 12 || day < 1 || day > 31 {
			return ""
		}
		return fmt.Sprintf("%d.%d.%d", year, month*100+day, build)
	default:
		return ""
	}
}

func NormalizeClawBoxInstalledTagVersion(raw string) string {
	normalized := normalizeClawBoxInstalledVersionInput(raw)
	if normalized == "" {
		return ""
	}

	parts := strings.Split(normalized, ".")
	switch len(parts) {
	case 4:
		year, okYear := parseClawBoxInstalledVersionPart(parts[0])
		month, okMonth := parseClawBoxInstalledVersionPart(parts[1])
		day, okDay := parseClawBoxInstalledVersionPart(parts[2])
		build, okBuild := parseClawBoxInstalledVersionPart(parts[3])
		if !okYear || !okMonth || !okDay || !okBuild {
			return ""
		}
		if month < 1 || month > 12 || day < 1 || day > 31 {
			return ""
		}
		return fmt.Sprintf("%d.%02d.%02d.%02d", year, month, day, build)
	case 3:
		year, okYear := parseClawBoxInstalledVersionPart(parts[0])
		minor, okMinor := parseClawBoxInstalledVersionPart(parts[1])
		build, okBuild := parseClawBoxInstalledVersionPart(parts[2])
		if !okYear || !okMinor || !okBuild {
			return ""
		}
		month := minor / 100
		day := minor % 100
		if month < 1 || month > 12 || day < 1 || day > 31 {
			return fmt.Sprintf("%d.%d.%d", year, minor, build)
		}
		return fmt.Sprintf("%d.%02d.%02d.%02d", year, month, day, build)
	default:
		return ""
	}
}

func cachedClawBoxInstalledRelease(repo string) (*clawBoxGitHubRelease, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		repo = defaultClawBoxInstalledRepo
	}
	now := time.Now()
	clawBoxInstalledReleaseCache.mu.RLock()
	entry, ok := clawBoxInstalledReleaseCache.items[repo]
	if ok && entry.release != nil && now.Sub(entry.fetchedAt) < clawBoxInstalledReleaseCacheTTL {
		release := entry.release
		clawBoxInstalledReleaseCache.mu.RUnlock()
		return release, nil
	}
	clawBoxInstalledReleaseCache.mu.RUnlock()

	release, err := fetchGitHubRelease(repo, "")
	if err != nil {
		return nil, fmt.Errorf("GitHub release 查询失败（repo=%s）: %w", repo, err)
	}

	clawBoxInstalledReleaseCache.mu.Lock()
	if clawBoxInstalledReleaseCache.items == nil {
		clawBoxInstalledReleaseCache.items = make(map[string]clawBoxInstalledReleaseCacheEntry)
	}
	clawBoxInstalledReleaseCache.items[repo] = clawBoxInstalledReleaseCacheEntry{
		release:   release,
		fetchedAt: time.Now(),
	}
	clawBoxInstalledReleaseCache.mu.Unlock()

	return release, nil
}

func buildClawBoxInstalledUpdateResponse(baseURL string, repo string, release *clawBoxGitHubRelease, currentVersion string) (*ClawBoxInstalledUpdateResponse, bool, error) {
	if release == nil {
		return nil, false, errors.New("ClawBox release data is unavailable")
	}

	releaseVersion := NormalizeClawBoxInstalledTagVersion(release.TagName)
	if releaseVersion == "" {
		return nil, false, fmt.Errorf("ClawBox release %s 不是合法的安装版标签", release.TagName)
	}
	latestVersion := NormalizeClawBoxInstalledSemver(release.TagName)
	if latestVersion == "" {
		return nil, false, fmt.Errorf("ClawBox release %s 无法转换为合法 semver", release.TagName)
	}
	if currentVersion != "" && portableVersionCompare(latestVersion, currentVersion) <= 0 {
		return nil, false, nil
	}

	installer := selectClawBoxInstalledAsset(release.Assets)
	if installer == nil {
		return nil, false, fmt.Errorf("ClawBox release %s missing Windows NSIS installer asset", release.TagName)
	}

	signatureAsset := findGitHubReleaseAsset(release.Assets, installer.Name+".sig")
	if signatureAsset == nil {
		return nil, false, fmt.Errorf("ClawBox release %s missing signature for %s", release.TagName, installer.Name)
	}

	signature, err := resolveClawBoxInstalledSignature(repo, release.TagName, signatureAsset)
	if err != nil {
		return nil, false, err
	}

	downloadURL := strings.TrimSpace(installer.BrowserDownloadURL)
	if downloadURL == "" {
		return nil, false, fmt.Errorf("ClawBox release %s installer asset lacks download URL", release.TagName)
	}
	if proxyURL := buildClawBoxInstalledProxyURL(baseURL, latestVersion, installer.Name); proxyURL != "" {
		downloadURL = proxyURL
	}

	pubDate := formatClawBoxReleasePublishedAt(release.PublishedAt)
	notes := strings.TrimSpace(release.Body)

	return &ClawBoxInstalledUpdateResponse{
		Version:   latestVersion,
		PubDate:   pubDate,
		URL:       downloadURL,
		Signature: signature,
		Notes:     notes,
	}, true, nil
}

func selectClawBoxInstalledAsset(assets []clawBoxGitHubReleaseAsset) *clawBoxGitHubReleaseAsset {
	var fallback *clawBoxGitHubReleaseAsset
	for i := range assets {
		if !isWindowsNsisInstallerAsset(&assets[i]) {
			continue
		}
		if findGitHubReleaseAsset(assets, assets[i].Name+".sig") == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(assets[i].Name))
		if strings.Contains(name, "setup") || strings.Contains(name, "nsis") {
			return &assets[i]
		}
		if fallback == nil {
			fallback = &assets[i]
		}
	}
	return fallback
}

func isWindowsNsisInstallerAsset(asset *clawBoxGitHubReleaseAsset) bool {
	if asset == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(asset.Name))
	if name == "" || !strings.HasSuffix(name, ".exe") {
		return false
	}
	if strings.HasSuffix(name, ".exe.sig") {
		return false
	}
	if strings.Contains(name, "portable") || strings.Contains(name, "updater") || strings.Contains(name, "debug") {
		return false
	}
	return true
}

func resolveClawBoxInstalledSignature(repo string, releaseTag string, asset *clawBoxGitHubReleaseAsset) (string, error) {
	if asset == nil {
		return "", errors.New("signature asset is missing")
	}
	cacheKey := strings.ToLower(fmt.Sprintf("%s#%s#%s", strings.TrimSpace(repo), releaseTag, strings.TrimSpace(asset.Name)))
	if cached, ok := clawBoxInstalledSignatureCache.Load(cacheKey); ok {
		if value, ok := cached.(string); ok && value != "" {
			return value, nil
		}
	}
	assetURL := firstNonEmpty(strings.TrimSpace(asset.URL), strings.TrimSpace(asset.BrowserDownloadURL))
	if assetURL == "" {
		return "", errors.New("signature asset download URL is missing")
	}
	bytes, err := fetchClawBoxPortableAssetBytes(assetURL, 4096)
	if err != nil {
		return "", err
	}
	signature := strings.TrimSpace(string(bytes))
	if signature == "" {
		return "", errors.New("signature asset returned empty content")
	}
	clawBoxInstalledSignatureCache.Store(cacheKey, signature)
	return signature, nil
}

func formatClawBoxReleasePublishedAt(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.Format(time.RFC3339)
	}
	return raw
}

func buildClawBoxInstalledProxyURL(baseURL string, version string, assetName string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	version = NormalizeClawBoxInstalledSemver(version)
	assetName = strings.TrimSpace(assetName)
	if baseURL == "" || version == "" {
		return ""
	}
	if assetName == "" {
		return fmt.Sprintf("%s/api/clawbox/update/desktop/releases/%s/download", baseURL, version)
	}
	values := neturl.Values{}
	values.Set("asset", assetName)
	return fmt.Sprintf("%s/api/clawbox/update/desktop/releases/%s/download?%s", baseURL, version, values.Encode())
}

func ResolveClawBoxInstalledPackageDownload(version string, assetName string) (*ClawBoxInstalledPackageDownload, error) {
	version = NormalizeClawBoxInstalledTagVersion(version)
	if version == "" {
		return nil, errors.New("安装版版本号不能为空")
	}

	repo := resolveClawBoxInstalledRepo()
	release, err := fetchGitHubRelease(repo, version)
	if err != nil {
		return nil, fmt.Errorf("GitHub release 查询失败（repo=%s version=%s）: %w", repo, version, err)
	}

	var installer *clawBoxGitHubReleaseAsset
	assetName = strings.TrimSpace(assetName)
	if assetName != "" {
		installer = findGitHubReleaseAsset(release.Assets, assetName)
		if installer == nil {
			return nil, fmt.Errorf("ClawBox release %s 缺少安装器资产 %s", release.TagName, assetName)
		}
		if !isWindowsNsisInstallerAsset(installer) {
			return nil, fmt.Errorf("ClawBox release %s 资产 %s 不是合法的 Windows NSIS 安装器", release.TagName, assetName)
		}
	} else {
		installer = selectClawBoxInstalledAsset(release.Assets)
		if installer == nil {
			return nil, fmt.Errorf("ClawBox release %s missing Windows NSIS installer asset", release.TagName)
		}
	}

	downloadURL := firstNonEmpty(strings.TrimSpace(installer.URL), strings.TrimSpace(installer.BrowserDownloadURL))
	if downloadURL == "" {
		return nil, fmt.Errorf("ClawBox release %s installer asset lacks download URL", release.TagName)
	}

	return &ClawBoxInstalledPackageDownload{
		AssetName:   strings.TrimSpace(installer.Name),
		DownloadURL: downloadURL,
		Source:      "github",
		Repo:        repo,
		Version:     NormalizeClawBoxInstalledSemver(release.TagName),
	}, nil
}

func OpenClawBoxInstalledPackageDownloadStream(pkg *ClawBoxInstalledPackageDownload, rangeHeader string) (*http.Response, error) {
	if pkg == nil {
		return nil, errors.New("安装包不存在")
	}

	url := strings.TrimSpace(pkg.DownloadURL)
	if url == "" {
		return nil, errors.New("安装包 download_url 为空")
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ClawBoxInstalledProxy/1.0")
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

func ClawBoxInstalledContentType(assetName string) string {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(assetName)), ".exe") {
		return "application/vnd.microsoft.portable-executable"
	}
	return "application/octet-stream"
}
