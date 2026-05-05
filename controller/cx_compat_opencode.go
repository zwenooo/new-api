package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"one-api/common"
	"one-api/model"

	promptdef "codex-service-go/prompt"

	"github.com/gin-gonic/gin"
)

const (
	cxCompatOpenCodeInstructionsOpt     = "cx_compat.opencode.instructions"
	cxCompatOpenCodeInstructionsMetaOpt = "cx_compat.opencode.instructions_meta"
	cxCompatOpenCodePinnedInstructions  = "cx_compat.opencode.pinned_instructions"
	cxCompatOpenCodePinnedMeta          = "cx_compat.opencode.pinned_meta"

	defaultOpenCodeGitHubRepo = "anomalyco/opencode"
	defaultOpenCodeGitHubRef  = "dev"
	defaultOpenCodeGitHubPath = "packages/opencode/src/session/prompt/codex_header.txt"
)

type cxCompatInstructionsMeta struct {
	Source       string `json:"source"`
	OriginSource string `json:"origin_source,omitempty"`

	// Local source
	LocalPath string `json:"local_path,omitempty"`

	// GitHub source
	Repo   string `json:"repo,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Path   string `json:"path,omitempty"`
	Commit string `json:"commit,omitempty"`

	// Audit
	SyncedAt   string `json:"synced_at,omitempty"`
	PinnedAt   string `json:"pinned_at,omitempty"`
	RestoredAt string `json:"restored_at,omitempty"`
	Sha256     string `json:"sha256,omitempty"`
}

var openCodeGitHubHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

func GetCxCompatOpenCodeInstructions(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	instructions := strings.TrimSpace(common.OptionMap[cxCompatOpenCodeInstructionsOpt])
	metaRaw := strings.TrimSpace(common.OptionMap[cxCompatOpenCodeInstructionsMetaOpt])
	pinnedRaw := strings.TrimSpace(common.OptionMap[cxCompatOpenCodePinnedMeta])
	common.OptionMapRWMutex.RUnlock()

	var meta any
	if metaRaw != "" {
		var parsed cxCompatInstructionsMeta
		if err := json.Unmarshal([]byte(metaRaw), &parsed); err == nil {
			meta = parsed
		}
	}

	var pinnedMeta any
	if pinnedRaw != "" {
		var parsed cxCompatInstructionsMeta
		if err := json.Unmarshal([]byte(pinnedRaw), &parsed); err == nil {
			pinnedMeta = parsed
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"instructions": instructions,
			"meta":         meta,
			"meta_raw":     metaRaw,
			"pinned_meta":  pinnedMeta,
			"pinned_raw":   pinnedRaw,
		},
	})
}

func SyncCxCompatOpenCodeInstructions(c *gin.Context) {
	path := resolveOpenCodeInstructionsPath()
	contents, err := os.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "读取 OpenCode instructions 文件失败: " + err.Error(),
		})
		return
	}
	instructions := strings.TrimSpace(string(contents))
	if instructions == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "OpenCode instructions 文件为空",
		})
		return
	}

	meta := cxCompatInstructionsMeta{
		Source:    "local",
		LocalPath: path,
		SyncedAt:  time.Now().UTC().Format(time.RFC3339),
		Sha256:    sha256Hex(instructions),
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "生成 OpenCode instructions meta 失败: " + err.Error(),
		})
		return
	}

	if err := model.UpdateOptionsAtomic(map[string]string{
		cxCompatOpenCodeInstructionsOpt:     instructions,
		cxCompatOpenCodeInstructionsMetaOpt: string(metaJSON),
	}); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "保存 OpenCode instructions 失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "同步成功（本机路径）",
		"data": gin.H{
			"instructions": instructions,
			"meta":         meta,
		},
	})
}

func SyncCxCompatOpenCodeInstructionsFromGitHub(c *gin.Context) {
	var req struct {
		Ref    string `json:"ref"`
		Commit string `json:"commit"`
	}
	_ = c.ShouldBindJSON(&req)

	repo, configuredRef, filePath := resolveOpenCodeGitHubSource()
	ref := strings.TrimSpace(coalesce(req.Ref, configuredRef))
	commit := strings.TrimSpace(req.Commit)
	if commit != "" {
		if err := validateGitHubCommitSHA(commit); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "commit 参数无效: " + err.Error(),
			})
			return
		}
		ref = commit
	} else if ref != "" {
		if err := validateGitHubRefName(ref); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "ref 参数无效: " + err.Error(),
			})
			return
		}
	}

	contents, err := fetchGitHubRaw(repo, ref, filePath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("从 GitHub 拉取 OpenCode instructions 失败（repo=%s ref=%s path=%s）: %s", repo, ref, filePath, err.Error()),
		})
		return
	}

	instructions := strings.TrimSpace(contents)
	if instructions == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "GitHub OpenCode instructions 内容为空",
		})
		return
	}

	commitSHA := ""
	var commitErr error
	if commit != "" {
		commitSHA = commit
	} else {
		commitSHA, commitErr = fetchGitHubLatestFileCommit(repo, ref, filePath)
	}
	meta := cxCompatInstructionsMeta{
		Source:   "github",
		Repo:     repo,
		Ref:      ref,
		Path:     filePath,
		Commit:   commitSHA,
		SyncedAt: time.Now().UTC().Format(time.RFC3339),
		Sha256:   sha256Hex(instructions),
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "生成 OpenCode instructions meta 失败: " + err.Error(),
		})
		return
	}
	if err := model.UpdateOptionsAtomic(map[string]string{
		cxCompatOpenCodeInstructionsOpt:     instructions,
		cxCompatOpenCodeInstructionsMetaOpt: string(metaJSON),
	}); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "保存 OpenCode instructions 失败: " + err.Error(),
		})
		return
	}

	message := "同步成功（GitHub）"
	if commitErr != nil {
		message += "（未获取 commit：" + commitErr.Error() + "）"
	} else if commitSHA != "" {
		short := commitSHA
		if len(short) > 8 {
			short = short[:8]
		}
		message += fmt.Sprintf(" %s@%s#%s", repo, ref, short)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
		"data": gin.H{
			"instructions": instructions,
			"meta":         meta,
		},
	})
}

func GetCxCompatOpenCodeGitHubBranches(c *gin.Context) {
	repo, configuredRef, filePath := resolveOpenCodeGitHubSource()

	defaultBranch, err := fetchGitHubRepoDefaultBranch(repo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取 GitHub 默认分支失败: " + err.Error(),
		})
		return
	}
	branches, err := fetchGitHubBranches(repo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取 GitHub 分支列表失败: " + err.Error(),
		})
		return
	}
	sort.Strings(branches)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"repo":           repo,
			"path":           filePath,
			"default_branch": defaultBranch,
			"configured_ref": configuredRef,
			"branches":       branches,
		},
	})
}

func GetCxCompatOpenCodeGitHubCommits(c *gin.Context) {
	repo, configuredRef, filePath := resolveOpenCodeGitHubSource()
	ref := strings.TrimSpace(c.Query("ref"))
	if ref == "" {
		ref = configuredRef
	}
	if ref == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "ref 参数不能为空",
		})
		return
	}
	if err := validateGitHubRefName(ref); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "ref 参数无效: " + err.Error(),
		})
		return
	}

	commits, err := fetchGitHubFileCommits(repo, ref, filePath, 20)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取 GitHub commits 失败（repo=%s ref=%s path=%s）: %s", repo, ref, filePath, err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"repo":    repo,
			"ref":     ref,
			"path":    filePath,
			"commits": commits,
		},
	})
}

func PinCxCompatOpenCodeInstructionsAsDefault(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	instructions := strings.TrimSpace(common.OptionMap[cxCompatOpenCodeInstructionsOpt])
	metaRaw := strings.TrimSpace(common.OptionMap[cxCompatOpenCodeInstructionsMetaOpt])
	common.OptionMapRWMutex.RUnlock()
	if instructions == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "当前 OpenCode instructions 为空，无法设为默认版本",
		})
		return
	}
	meta, err := parseCxCompatInstructionsMeta(metaRaw)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "当前 OpenCode instructions meta 无效，无法设为默认版本: " + err.Error(),
		})
		return
	}
	meta.PinnedAt = time.Now().UTC().Format(time.RFC3339)
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "生成默认版本 meta 失败: " + err.Error(),
		})
		return
	}
	if err := model.UpdateOptionsAtomic(map[string]string{
		cxCompatOpenCodePinnedInstructions: instructions,
		cxCompatOpenCodePinnedMeta:         string(metaJSON),
	}); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "设为默认版本失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已设为默认版本",
		"data": gin.H{
			"meta": meta,
		},
	})
}

func RestoreCxCompatOpenCodeInstructionsDefault(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	pinned := strings.TrimSpace(common.OptionMap[cxCompatOpenCodePinnedInstructions])
	pinnedMetaRaw := strings.TrimSpace(common.OptionMap[cxCompatOpenCodePinnedMeta])
	common.OptionMapRWMutex.RUnlock()

	now := time.Now().UTC().Format(time.RFC3339)
	if pinned != "" {
		meta, err := parseCxCompatInstructionsMeta(pinnedMetaRaw)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "默认版本 meta 无效，无法恢复: " + err.Error(),
			})
			return
		}
		meta.OriginSource = meta.Source
		meta.Source = "pinned_default"
		meta.RestoredAt = now
		metaJSON, err := json.Marshal(meta)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "生成恢复 meta 失败: " + err.Error(),
			})
			return
		}
		if err := model.UpdateOptionsAtomic(map[string]string{
			cxCompatOpenCodeInstructionsOpt:     pinned,
			cxCompatOpenCodeInstructionsMetaOpt: string(metaJSON),
		}); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "恢复默认版本失败: " + err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "已恢复到默认版本（已固定）",
			"data": gin.H{
				"instructions": pinned,
				"meta":         meta,
			},
		})
		return
	}

	instructions := strings.TrimSpace(promptdef.OpenCodeCodexHeader)
	if instructions == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "内置 OpenCode instructions 为空，无法恢复",
		})
		return
	}

	meta := cxCompatInstructionsMeta{
		Source:     "builtin",
		RestoredAt: now,
		Sha256:     sha256Hex(instructions),
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "生成恢复 meta 失败: " + err.Error(),
		})
		return
	}
	if err := model.UpdateOptionsAtomic(map[string]string{
		cxCompatOpenCodeInstructionsOpt:     instructions,
		cxCompatOpenCodeInstructionsMetaOpt: string(metaJSON),
	}); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "恢复内置默认失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已恢复到内置默认版本",
		"data": gin.H{
			"instructions": instructions,
			"meta":         meta,
		},
	})
}

func resolveOpenCodeInstructionsPath() string {
	if v := strings.TrimSpace(os.Getenv("ONEAPI_CX_COMPAT_OPENCODE_INSTRUCTIONS_PATH")); v != "" {
		return v
	}
	return filepath.Clean(filepath.Join("..", "opencode", "packages", "opencode", "src", "session", "prompt", "codex_header.txt"))
}

func resolveOpenCodeGitHubSource() (repo string, ref string, filePath string) {
	repo = strings.TrimSpace(os.Getenv("ONEAPI_CX_COMPAT_OPENCODE_GITHUB_REPO"))
	if repo == "" {
		repo = defaultOpenCodeGitHubRepo
	}
	ref = strings.TrimSpace(os.Getenv("ONEAPI_CX_COMPAT_OPENCODE_GITHUB_REF"))
	if ref == "" {
		ref = defaultOpenCodeGitHubRef
	}
	filePath = strings.TrimSpace(os.Getenv("ONEAPI_CX_COMPAT_OPENCODE_GITHUB_PATH"))
	if filePath == "" {
		filePath = defaultOpenCodeGitHubPath
	}
	return repo, ref, filePath
}

func resolveGitHubToken() string {
	token := strings.TrimSpace(os.Getenv("ONEAPI_CX_COMPAT_OPENCODE_GITHUB_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("ONEAPI_CX_COMPAT_GITHUB_TOKEN"))
	}
	return token
}

func applyGitHubAuthHeader(req *http.Request) {
	token := resolveGitHubToken()
	if token == "" {
		return
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		req.Header.Set("Authorization", token)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}

func fetchGitHubRaw(repo string, ref string, filePath string) (string, error) {
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", repo, ref, filePath)
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "one-api")
	applyGitHubAuthHeader(req)
	resp, err := openCodeGitHubHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return string(body), nil
}

func fetchGitHubLatestFileCommit(repo string, ref string, filePath string) (string, error) {
	commits, err := fetchGitHubFileCommits(repo, ref, filePath, 1)
	if err != nil {
		return "", err
	}
	if len(commits) == 0 || strings.TrimSpace(commits[0].SHA) == "" {
		return "", fmt.Errorf("empty commits response")
	}
	return strings.TrimSpace(commits[0].SHA), nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

type cxCompatGitHubCommit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Date    string `json:"date"`
}

func fetchGitHubAPIJSON(u string, limitBytes int64) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "one-api")
	applyGitHubAuthHeader(req)
	resp, err := openCodeGitHubHTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if limitBytes <= 0 {
		limitBytes = 1024 * 1024
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limitBytes))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode != http.StatusOK {
		return body, resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, resp.StatusCode, nil
}

func fetchGitHubRepoDefaultBranch(repo string) (string, error) {
	u := url.URL{
		Scheme: "https",
		Host:   "api.github.com",
		Path:   fmt.Sprintf("/repos/%s", repo),
	}
	body, _, err := fetchGitHubAPIJSON(u.String(), 512*1024)
	if err != nil {
		return "", err
	}
	var out struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.DefaultBranch) == "" {
		return "", fmt.Errorf("default_branch is empty")
	}
	return strings.TrimSpace(out.DefaultBranch), nil
}

func fetchGitHubBranches(repo string) ([]string, error) {
	u := url.URL{
		Scheme: "https",
		Host:   "api.github.com",
		Path:   fmt.Sprintf("/repos/%s/branches", repo),
	}
	q := u.Query()
	q.Set("per_page", "100")
	u.RawQuery = q.Encode()

	body, _, err := fetchGitHubAPIJSON(u.String(), 1024*1024)
	if err != nil {
		return nil, err
	}
	var branches []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &branches); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(branches))
	for _, b := range branches {
		name := strings.TrimSpace(b.Name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}

func fetchGitHubFileCommits(repo string, ref string, filePath string, limit int) ([]cxCompatGitHubCommit, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	u := url.URL{
		Scheme: "https",
		Host:   "api.github.com",
		Path:   fmt.Sprintf("/repos/%s/commits", repo),
	}
	q := u.Query()
	q.Set("path", filePath)
	q.Set("sha", ref)
	q.Set("per_page", fmt.Sprintf("%d", limit))
	u.RawQuery = q.Encode()

	body, _, err := fetchGitHubAPIJSON(u.String(), 2*1024*1024)
	if err != nil {
		return nil, err
	}
	var commits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Date string `json:"date"`
			} `json:"author"`
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(body, &commits); err != nil {
		return nil, err
	}
	out := make([]cxCompatGitHubCommit, 0, len(commits))
	for _, c := range commits {
		sha := strings.TrimSpace(c.SHA)
		if sha == "" {
			continue
		}
		message := strings.TrimSpace(c.Commit.Message)
		date := strings.TrimSpace(c.Commit.Author.Date)
		if date == "" {
			date = strings.TrimSpace(c.Commit.Committer.Date)
		}
		out = append(out, cxCompatGitHubCommit{
			SHA:     sha,
			Message: message,
			Date:    date,
		})
	}
	return out, nil
}

func validateGitHubCommitSHA(sha string) error {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return fmt.Errorf("commit is empty")
	}
	if len(sha) < 7 || len(sha) > 40 {
		return fmt.Errorf("commit length must be 7-40")
	}
	for _, r := range sha {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return fmt.Errorf("commit must be hex")
	}
	return nil
}

func validateGitHubRefName(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("ref is empty")
	}
	if len(ref) > 255 {
		return fmt.Errorf("ref is too long")
	}
	if strings.ContainsAny(ref, " \t\r\n\\") {
		return fmt.Errorf("ref contains invalid characters")
	}
	if strings.Contains(ref, "..") {
		return fmt.Errorf("ref contains '..'")
	}
	if strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, "/") {
		return fmt.Errorf("ref must not start/end with '/'")
	}
	return nil
}

func parseCxCompatInstructionsMeta(metaRaw string) (cxCompatInstructionsMeta, error) {
	if strings.TrimSpace(metaRaw) == "" {
		return cxCompatInstructionsMeta{}, fmt.Errorf("meta is empty")
	}
	var meta cxCompatInstructionsMeta
	if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
		return cxCompatInstructionsMeta{}, err
	}
	if strings.TrimSpace(meta.Source) == "" {
		return cxCompatInstructionsMeta{}, fmt.Errorf("meta.source is empty")
	}
	return meta, nil
}
