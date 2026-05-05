package model

import (
	"encoding/json"
	"errors"
	"net/url"
	"one-api/common"
	"regexp"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ClawBoxPortableRepoOptionKey    = "ClawBoxPortableRepo"
	ClawBoxPortableChannelOptionKey = "ClawBoxPortableChannel"
	ClawBoxPortableGitHubTokenKey   = "ClawBoxPortableGitHubToken"
	ClawBoxPortableUpdateEnabledKey = "ClawBoxPortableUpdateEnabled"
	clawBoxPortableDefaultMode      = "portable"
	clawBoxPortableDefaultPlatform  = "windows"
	clawBoxPortableDefaultArch      = "x64"
	clawBoxPortableDefaultChannel   = "stable"
)

var clawBoxPortableProxyDownloadPathPattern = regexp.MustCompile(`^/api/clawbox/update/portable/releases/\d+/download/?$`)

type ClawBoxPortableRelease struct {
	Id             int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Enabled        bool   `json:"enabled" gorm:"not null;default:true;index"`
	IsLatest       bool   `json:"is_latest" gorm:"not null;default:false;index"`
	Version        string `json:"version" gorm:"type:varchar(64);not null;index"`
	Tag            string `json:"tag" gorm:"type:varchar(128);not null;default:''"`
	Mode           string `json:"mode" gorm:"type:varchar(32);not null;default:'portable';index:idx_clawbox_portable_selector"`
	Platform       string `json:"platform" gorm:"type:varchar(32);not null;default:'windows';index:idx_clawbox_portable_selector"`
	Arch           string `json:"arch" gorm:"type:varchar(32);not null;default:'x64';index:idx_clawbox_portable_selector"`
	Channel        string `json:"channel" gorm:"type:varchar(32);not null;default:'stable';index:idx_clawbox_portable_selector"`
	Source         string `json:"source" gorm:"type:varchar(32);not null;default:'github';index"`
	Repo           string `json:"repo" gorm:"type:varchar(255);not null;default:''"`
	AssetName      string `json:"asset_name" gorm:"type:varchar(255);not null;default:''"`
	DownloadUrl    string `json:"download_url" gorm:"type:text;not null"`
	DownloadSha256 string `json:"download_sha256" gorm:"type:varchar(128);not null;default:''"`
	ReleasePageUrl string `json:"release_page_url" gorm:"type:text;not null"`
	ReleaseNotes   string `json:"release_notes" gorm:"type:text"`
	MinAppVersion  string `json:"min_app_version" gorm:"type:varchar(64);not null;default:'0.1.0'"`
	PublishedTime  int64  `json:"published_time" gorm:"not null;default:0;index"`
	CreatedTime    int64  `json:"created_time" gorm:"autoCreateTime"`
	UpdatedTime    int64  `json:"updated_time" gorm:"autoUpdateTime"`
	SourceMetaRaw  string `json:"-" gorm:"type:text"`
}

type ClawBoxPortableReleaseUpsertParams struct {
	SetLatest      bool
	Enabled        bool
	Version        string
	Tag            string
	Mode           string
	Platform       string
	Arch           string
	Channel        string
	Source         string
	Repo           string
	AssetName      string
	DownloadUrl    string
	DownloadSha256 string
	ReleasePageUrl string
	ReleaseNotes   string
	MinAppVersion  string
	PublishedTime  int64
	SourceMeta     map[string]interface{}
}

func (r *ClawBoxPortableRelease) SourceMeta() map[string]interface{} {
	if strings.TrimSpace(r.SourceMetaRaw) == "" {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(r.SourceMetaRaw), &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func NormalizeClawBoxPortableReleaseSelector(mode string, platform string, arch string, channel string) (string, string, string, string) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		mode = clawBoxPortableDefaultMode
	}
	platform = strings.TrimSpace(strings.ToLower(platform))
	switch platform {
	case "", "windows", "win", "win32", "win64":
		platform = clawBoxPortableDefaultPlatform
	case "mac", "macos", "osx", "macosx", "darwin":
		platform = "macos"
	case "gnu/linux":
		platform = "linux"
	}
	arch = strings.TrimSpace(strings.ToLower(arch))
	switch arch {
	case "", "x64", "amd64", "x86_64", "x86-64":
		arch = clawBoxPortableDefaultArch
	case "aarch64":
		arch = "arm64"
	}
	channel = strings.TrimSpace(strings.ToLower(channel))
	switch channel {
	case "", "stable", "release", "latest":
		channel = clawBoxPortableDefaultChannel
	}
	return mode, platform, arch, channel
}

func IsClawBoxPortableProxyDownloadURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}

	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" {
		path = strings.TrimSpace(parsed.Path)
	}
	if path == "" {
		return false
	}

	return clawBoxPortableProxyDownloadPathPattern.MatchString(path)
}

func ClawBoxPortableUpdateEnabled() bool {
	common.OptionMapRWMutex.RLock()
	value := strings.TrimSpace(common.OptionMap[ClawBoxPortableUpdateEnabledKey])
	common.OptionMapRWMutex.RUnlock()
	if value == "" {
		return true
	}
	return strings.EqualFold(value, "true")
}

func validateClawBoxPortableDownloadURL(raw string) error {
	downloadURL := strings.TrimSpace(raw)
	if downloadURL == "" {
		return errors.New("download_url 不能为空")
	}

	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return errors.New("download_url 必须是完整的 http/https 地址")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("download_url 必须是完整的 http/https 地址")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return errors.New("download_url 必须包含主机名")
	}
	if IsClawBoxPortableProxyDownloadURL(downloadURL) {
		return errors.New("download_url 不能指向 new-api 自己的 Portable 代理下载地址")
	}
	return nil
}

func ListClawBoxPortableReleases(mode string, platform string, arch string, channel string) ([]*ClawBoxPortableRelease, error) {
	mode, platform, arch, channel = NormalizeClawBoxPortableReleaseSelector(mode, platform, arch, channel)
	var releases []*ClawBoxPortableRelease
	err := DB.
		Where("mode = ? AND platform = ? AND arch = ? AND channel = ?", mode, platform, arch, channel).
		Order("is_latest DESC").
		Order("published_time DESC").
		Order("id DESC").
		Find(&releases).Error
	if err != nil {
		return nil, err
	}
	return dedupeClawBoxPortableReleases(releases), nil
}

func GetLatestClawBoxPortableRelease(mode string, platform string, arch string, channel string) (*ClawBoxPortableRelease, error) {
	mode, platform, arch, channel = NormalizeClawBoxPortableReleaseSelector(mode, platform, arch, channel)

	var releases []*ClawBoxPortableRelease
	err := DB.
		Where("mode = ? AND platform = ? AND arch = ? AND channel = ? AND enabled = ?", mode, platform, arch, channel, true).
		Order("is_latest DESC").
		Order("published_time DESC").
		Order("id DESC").
		Find(&releases).Error
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	preferred := releases[0]
	latestCount := 0
	for i := range releases {
		if releases[i] != nil && releases[i].IsLatest {
			latestCount++
		}
	}

	needRepairLatest := latestCount != 1 || !preferred.IsLatest
	if needRepairLatest && preferred.Id > 0 {
		if repairErr := setClawBoxPortableLatestBySelector(nil, preferred.Id, mode, platform, arch, channel); repairErr != nil {
			common.SysError("repair clawbox portable latest failed: " + repairErr.Error())
		} else {
			preferred.IsLatest = true
		}
	}
	return preferred, nil
}

func GetClawBoxPortableReleaseByID(id int64) (*ClawBoxPortableRelease, error) {
	if id <= 0 {
		return nil, errors.New("无效的版本 ID")
	}
	var release ClawBoxPortableRelease
	if err := DB.Where("id = ?", id).First(&release).Error; err != nil {
		return nil, err
	}
	return &release, nil
}

func ActivateClawBoxPortableRelease(id int64) (*ClawBoxPortableRelease, error) {
	if id <= 0 {
		return nil, errors.New("无效的版本 ID")
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var release ClawBoxPortableRelease
		targetQuery := tx
		if !common.UsingSQLite {
			targetQuery = targetQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := targetQuery.
			Where("id = ?", id).
			First(&release).Error; err != nil {
			return err
		}

		mode, platform, arch, channel := NormalizeClawBoxPortableReleaseSelector(release.Mode, release.Platform, release.Arch, release.Channel)
		if err := tx.Model(&ClawBoxPortableRelease{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"enabled":  true,
				"mode":     mode,
				"platform": platform,
				"arch":     arch,
				"channel":  channel,
			}).Error; err != nil {
			return err
		}
		if err := setClawBoxPortableLatestBySelector(tx, id, mode, platform, arch, channel); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return GetClawBoxPortableReleaseByID(id)
}

func DeleteClawBoxPortableRelease(id int64) error {
	if id <= 0 {
		return errors.New("无效的版本 ID")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var release ClawBoxPortableRelease
		targetQuery := tx
		if !common.UsingSQLite {
			targetQuery = targetQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := targetQuery.Where("id = ?", id).First(&release).Error; err != nil {
			return err
		}

		mode, platform, arch, channel := NormalizeClawBoxPortableReleaseSelector(
			release.Mode,
			release.Platform,
			release.Arch,
			release.Channel,
		)

		if err := tx.Delete(&ClawBoxPortableRelease{}, "id = ?", id).Error; err != nil {
			return err
		}

		var nextLatest ClawBoxPortableRelease
		nextQuery := tx.Model(&ClawBoxPortableRelease{}).
			Where("mode = ? AND platform = ? AND arch = ? AND channel = ? AND enabled = ?", mode, platform, arch, channel, true).
			Order("is_latest DESC").
			Order("published_time DESC").
			Order("id DESC")
		if !common.UsingSQLite {
			nextQuery = nextQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := nextQuery.First(&nextLatest).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		return setClawBoxPortableLatestBySelector(tx, nextLatest.Id, mode, platform, arch, channel)
	})
}

func UpsertClawBoxPortableRelease(params ClawBoxPortableReleaseUpsertParams) (*ClawBoxPortableRelease, error) {
	mode, platform, arch, channel := NormalizeClawBoxPortableReleaseSelector(params.Mode, params.Platform, params.Arch, params.Channel)
	version := strings.TrimSpace(params.Version)
	if version == "" {
		return nil, errors.New("version 不能为空")
	}
	downloadURL := strings.TrimSpace(params.DownloadUrl)
	if err := validateClawBoxPortableDownloadURL(downloadURL); err != nil {
		return nil, err
	}
	downloadSHA256 := strings.TrimSpace(strings.ToLower(strings.TrimPrefix(params.DownloadSha256, "sha256:")))
	if downloadSHA256 == "" {
		return nil, errors.New("download_sha256 不能为空")
	}
	assetName := strings.TrimSpace(params.AssetName)
	if assetName == "" {
		return nil, errors.New("asset_name 不能为空")
	}
	source := strings.TrimSpace(strings.ToLower(params.Source))
	if source == "" {
		source = "github"
	}
	repo := strings.TrimSpace(params.Repo)
	if repo == "" {
		repo = GetClawBoxPortableRepo()
	}
	minAppVersion := strings.TrimSpace(params.MinAppVersion)
	if minAppVersion == "" {
		minAppVersion = "0.1.0"
	}
	sourceMetaRaw := ""
	if len(params.SourceMeta) > 0 {
		if data, err := json.Marshal(params.SourceMeta); err == nil {
			sourceMetaRaw = string(data)
		}
	}

	release := &ClawBoxPortableRelease{}
	err := DB.
		Where("version = ? AND mode = ? AND platform = ? AND arch = ? AND channel = ? AND source = ?", version, mode, platform, arch, channel, source).
		First(release).Error

	switch {
	case err == nil:
		release.Enabled = params.Enabled
		release.Tag = normalizeClawBoxPortableTag(strings.TrimSpace(params.Tag), version)
		release.Repo = repo
		release.AssetName = assetName
		release.DownloadUrl = downloadURL
		release.DownloadSha256 = downloadSHA256
		release.ReleasePageUrl = strings.TrimSpace(params.ReleasePageUrl)
		release.ReleaseNotes = strings.TrimSpace(params.ReleaseNotes)
		release.MinAppVersion = minAppVersion
		if params.PublishedTime > 0 {
			release.PublishedTime = params.PublishedTime
		}
		if sourceMetaRaw != "" {
			release.SourceMetaRaw = sourceMetaRaw
		}
		if err := DB.Save(release).Error; err != nil {
			return nil, err
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		release = &ClawBoxPortableRelease{
			Enabled:        params.Enabled,
			IsLatest:       false,
			Version:        version,
			Tag:            normalizeClawBoxPortableTag(strings.TrimSpace(params.Tag), version),
			Mode:           mode,
			Platform:       platform,
			Arch:           arch,
			Channel:        channel,
			Source:         source,
			Repo:           repo,
			AssetName:      assetName,
			DownloadUrl:    downloadURL,
			DownloadSha256: downloadSHA256,
			ReleasePageUrl: strings.TrimSpace(params.ReleasePageUrl),
			ReleaseNotes:   strings.TrimSpace(params.ReleaseNotes),
			MinAppVersion:  minAppVersion,
			PublishedTime:  params.PublishedTime,
			SourceMetaRaw:  sourceMetaRaw,
		}
		if err := DB.Create(release).Error; err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	if params.SetLatest {
		return ActivateClawBoxPortableRelease(release.Id)
	}
	return GetClawBoxPortableReleaseByID(release.Id)
}

func GetClawBoxPortableRepo() string {
	common.OptionMapRWMutex.RLock()
	repo := strings.TrimSpace(common.OptionMap[ClawBoxPortableRepoOptionKey])
	common.OptionMapRWMutex.RUnlock()
	if repo == "" {
		return "zwenooo/ClawBox"
	}
	return repo
}

func normalizeClawBoxPortableTag(tag string, version string) string {
	tag = strings.TrimSpace(tag)
	if tag != "" {
		return tag
	}
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if version == "" {
		return ""
	}
	return "v" + version
}

func setClawBoxPortableLatestBySelector(tx *gorm.DB, id int64, mode string, platform string, arch string, channel string) error {
	if id <= 0 {
		return errors.New("无效的版本 ID")
	}
	mode, platform, arch, channel = NormalizeClawBoxPortableReleaseSelector(mode, platform, arch, channel)

	db := DB
	if tx != nil {
		db = tx
	}
	return db.Model(&ClawBoxPortableRelease{}).
		Where("mode = ? AND platform = ? AND arch = ? AND channel = ?", mode, platform, arch, channel).
		Update("is_latest", gorm.Expr("id = ?", id)).Error
}

func dedupeClawBoxPortableReleases(releases []*ClawBoxPortableRelease) []*ClawBoxPortableRelease {
	if len(releases) <= 1 {
		return releases
	}
	seen := make(map[string]struct{}, len(releases))
	out := make([]*ClawBoxPortableRelease, 0, len(releases))
	for i := range releases {
		release := releases[i]
		if release == nil {
			continue
		}
		mode, platform, arch, channel := NormalizeClawBoxPortableReleaseSelector(release.Mode, release.Platform, release.Arch, release.Channel)
		releaseKeyParts := []string{
			strings.ToLower(strings.TrimSpace(release.Version)),
			mode,
			platform,
			arch,
			channel,
			strings.ToLower(strings.TrimSpace(release.Source)),
		}
		containsEmpty := false
		for _, part := range releaseKeyParts {
			if part == "" {
				containsEmpty = true
				break
			}
		}
		if containsEmpty {
			out = append(out, release)
			continue
		}
		key := strings.Join(releaseKeyParts, "\x1f")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, release)
	}
	return out
}
