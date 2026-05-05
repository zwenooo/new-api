package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	sshPort         = 2222
	dropInDir       = "/etc/ssh/sshd_config.d"
	dropInFile      = dropInDir + "/zz-codex-2222.conf"
	mainSSHConfig   = "/etc/ssh/sshd_config"
	backupSSHConfig = "/etc/ssh/sshd_config.codex.bak"
)

// Service exposes helpers for managing SSH relay requirements (firewall/sshd tweaks).
type Service struct{}

func NewService() *Service {
	return &Service{}
}

type SSHStatus struct {
	FirewallOpen bool
	Listening    bool
	FirewallTool string
	Enabled      bool
	Errors       []string
}

func (s *Service) Status(ctx context.Context) SSHStatus {
	status := SSHStatus{}
	open, tool, err := isFirewallPortOpen(ctx, sshPort)
	if err != nil {
		status.Errors = append(status.Errors, err.Error())
	} else {
		status.FirewallOpen = open
		status.FirewallTool = tool
	}
	listening, err := isPortListening(ctx, sshPort)
	if err != nil {
		status.Errors = append(status.Errors, err.Error())
	} else {
		status.Listening = listening
	}
	status.Enabled = detectSSHConfig()
	return status
}

func (s *Service) SetFirewall(ctx context.Context, enable bool) error {
	if enable {
		return openFirewallPort(ctx, sshPort)
	}
	return closeFirewallPort(ctx, sshPort)
}

type SSHConfigResult struct {
	Listening bool
}

func (s *Service) EnableSSH(ctx context.Context) (SSHConfigResult, error) {
	result := SSHConfigResult{}
	usedDropIn := false
	rollback := func() {
		if usedDropIn {
			_ = os.Remove(dropInFile)
		} else {
			if _, err := os.Stat(backupSSHConfig); err == nil {
				_ = copyFile(backupSSHConfig, mainSSHConfig)
			}
		}
	}

	if _, err := os.Stat(dropInDir); err == nil {
		hasPort22 := hasPortInConfig(22)
		content := "# codex-admin enable 2222\nPort 2222\n"
		if !hasPort22 {
			content = "# codex-admin enable 2222\nPort 22\nPort 2222\n"
		}
		if err := os.WriteFile(dropInFile, []byte(content), 0o644); err != nil {
			return result, err
		}
		usedDropIn = true
	} else {
		if _, err := os.Stat(backupSSHConfig); errors.Is(err, os.ErrNotExist) {
			if err := copyFile(mainSSHConfig, backupSSHConfig); err != nil && !errors.Is(err, os.ErrNotExist) {
				return result, err
			}
		}
		data, _ := os.ReadFile(mainSSHConfig)
		uncommented := stripComments(string(data))
		if !strings.Contains(uncommented, "Port 2222") {
			builder := strings.Builder{}
			builder.Write(data)
			if !strings.Contains(uncommented, "Port 22") {
				builder.WriteString("\n# codex-admin ensure 22\nPort 22\n")
			}
			builder.WriteString("\n# codex-admin enable 2222\nPort 2222\n")
			if err := os.WriteFile(mainSSHConfig, []byte(builder.String()), 0o644); err != nil {
				return result, err
			}
		}
	}

	if ok := testSshdConfig(ctx); !ok {
		rollback()
		return result, errors.New("sshd_config_invalid")
	}
	_ = reloadSsh(ctx)
	listening, err := isPortListening(ctx, sshPort)
	if err != nil {
		return result, err
	}
	result.Listening = listening
	return result, nil
}

func (s *Service) DisableSSH(ctx context.Context) (SSHConfigResult, error) {
	result := SSHConfigResult{}
	if _, err := os.Stat(dropInFile); err == nil {
		_ = os.Remove(dropInFile)
	} else {
		data, err := os.ReadFile(mainSSHConfig)
		if err == nil {
			updated := stripCodexBlock(string(data))
			if updated != string(data) {
				if err := os.WriteFile(mainSSHConfig, []byte(updated), 0o644); err != nil {
					return result, err
				}
			}
		}
	}
	if ok := testSshdConfig(ctx); !ok {
		if _, err := os.Stat(backupSSHConfig); err == nil {
			_ = copyFile(backupSSHConfig, mainSSHConfig)
		}
		return result, errors.New("sshd_config_invalid")
	}
	_ = reloadSsh(ctx)
	listening, err := isPortListening(ctx, sshPort)
	if err != nil {
		return result, err
	}
	result.Listening = listening
	return result, nil
}

func (s *Service) KillPort(ctx context.Context, port int) ([]int, error) {
	pids, err := pidsByPort(ctx, port)
	if err != nil {
		return nil, err
	}
	if len(pids) == 0 {
		return nil, nil
	}
	killed := make([]int, 0, len(pids))
	for _, pid := range pids {
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		_ = proc.Signal(os.Interrupt)
		killed = append(killed, pid)
	}
	time.Sleep(200 * time.Millisecond)
	for _, pid := range pids {
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		_ = proc.Kill()
	}
	return killed, nil
}

// --- helpers ---

func runCommand(ctx context.Context, cmd string, args ...string) (string, string, error) {
	command := exec.CommandContext(ctx, cmd, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func isFirewallPortOpen(ctx context.Context, port int) (bool, string, error) {
	tool := detectFirewallTool()
	switch tool {
	case "netsh":
		// Windows: 查询规则是否存在（规则名：codex-svc-<port>）
		rule := fmt.Sprintf("codex-svc-%d", port)
		stdout, stderr, err := runCommand(ctx, "netsh", "advfirewall", "firewall", "show", "rule", fmt.Sprintf("name=%s", rule))
		if err != nil {
			// netsh 返回码在不同系统可能仍为 0，这里主要靠输出判断
			_ = err
		}
		out := strings.ToLower(stdout + "\n" + stderr)
		if strings.Contains(out, strings.ToLower(rule)) {
			return true, tool, nil
		}
		// 兼容英文/中文“不存在”提示：不出现规则名视为未开启
		return false, tool, nil
	case "firewalld":
		stdout, _, err := runCommand(ctx, "firewall-cmd", "--query-port", fmt.Sprintf("%d/tcp", port))
		if err != nil {
			return false, tool, err
		}
		return strings.Contains(strings.ToLower(stdout), "yes"), tool, nil
	case "ufw":
		stdout, stderr, err := runCommand(ctx, "ufw", "status")
		if err != nil {
			// ufw status 会在非 root 下提示权限问题，需要返回错误便于前端展示
			if strings.Contains(strings.ToLower(stderr), "need to be root") {
				return false, tool, errors.New("ufw requires root privileges")
			}
			// 如果命令执行失败但仍然有输出，继续尝试解析
		}
		open := parseUFWStatus(stdout, port)
		return open, tool, nil
	case "iptables":
		_, _, err := runCommand(ctx, "sh", "-lc", fmt.Sprintf("iptables -C INPUT -p tcp --dport %d -j ACCEPT >/dev/null 2>&1", port))
		if err == nil {
			return true, tool, nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			return false, tool, nil
		}
		return false, tool, err
	default:
		return false, tool, nil
	}
}

func openFirewallPort(ctx context.Context, port int) error {
	switch detectFirewallTool() {
	case "netsh":
		// 先删除同名规则，再添加放行规则
		rule := fmt.Sprintf("codex-svc-%d", port)
		_, _, _ = runCommand(ctx, "netsh", "advfirewall", "firewall", "delete", "rule", fmt.Sprintf("name=%s", rule), "protocol=TCP", fmt.Sprintf("localport=%d", port))
		_, _, err := runCommand(ctx, "netsh", "advfirewall", "firewall", "add", "rule",
			fmt.Sprintf("name=%s", rule), "dir=in", "action=allow", "protocol=TCP", fmt.Sprintf("localport=%d", port))
		return err
	case "firewalld":
		if _, _, err := runCommand(ctx, "firewall-cmd", "--permanent", fmt.Sprintf("--add-port=%d/tcp", port)); err != nil {
			return err
		}
		_, _, err := runCommand(ctx, "firewall-cmd", "--reload")
		return err
	case "ufw":
		_, _, err := runCommand(ctx, "ufw", "allow", fmt.Sprintf("%d/tcp", port))
		return err
	case "iptables":
		if open, _, err := isFirewallPortOpen(ctx, port); err == nil && open {
			return nil
		}
		_, _, err := runCommand(ctx, "iptables", "-I", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT")
		return err
	default:
		return nil
	}
}

func closeFirewallPort(ctx context.Context, port int) error {
	switch detectFirewallTool() {
	case "netsh":
		rule := fmt.Sprintf("codex-svc-%d", port)
		_, _, err := runCommand(ctx, "netsh", "advfirewall", "firewall", "delete", "rule", fmt.Sprintf("name=%s", rule), "protocol=TCP", fmt.Sprintf("localport=%d", port))
		return err
	case "firewalld":
		if _, _, err := runCommand(ctx, "firewall-cmd", "--permanent", fmt.Sprintf("--remove-port=%d/tcp", port)); err != nil {
			return err
		}
		_, _, err := runCommand(ctx, "firewall-cmd", "--reload")
		return err
	case "ufw":
		_, _, err := runCommand(ctx, "ufw", "delete", "allow", fmt.Sprintf("%d/tcp", port))
		return err
	case "iptables":
		_, _, err := runCommand(ctx, "iptables", "-D", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT")
		return err
	default:
		return nil
	}
}

func detectFirewallTool() string {
	// Windows 优先
	if runtime.GOOS == "windows" && commandExists("netsh") {
		return "netsh"
	}
	if commandExists("firewall-cmd") {
		return "firewalld"
	}
	if commandExists("ufw") {
		return "ufw"
	}
	if commandExists("iptables") {
		return "iptables"
	}
	return ""
}

func isPortListening(ctx context.Context, port int) (bool, error) {
	if commandExists("ss") {
		_, _, err := runCommand(ctx, "sh", "-lc", fmt.Sprintf("ss -ltnp 2>/dev/null | grep -q ':%d '", port))
		if err == nil {
			return true, nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			// not listening
			return false, nil
		}
	}
	if commandExists("netstat") {
		_, _, err := runCommand(ctx, "sh", "-lc", fmt.Sprintf("netstat -tlnp 2>/dev/null | grep -q ':%d '", port))
		if err == nil {
			return true, nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			return false, nil
		}
	}
	return false, nil
}

func reloadSsh(ctx context.Context) bool {
	cmd := "systemctl reload ssh || systemctl reload sshd || service ssh reload || service sshd reload || true"
	_, _, err := runCommand(ctx, "sh", "-lc", cmd)
	return err == nil
}

func testSshdConfig(ctx context.Context) bool {
	cmd := "sshd -t 2>/dev/null || /usr/sbin/sshd -t 2>/dev/null"
	_, _, err := runCommand(ctx, "sh", "-lc", cmd)
	return err == nil
}

func stripComments(data string) string {
	lines := strings.Split(data, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func stripCodexBlock(data string) string {
	replacer := strings.NewReplacer(
		"\n# codex-admin enable 2222\nPort 2222\n", "\n",
		"# codex-admin enable 2222\nPort 2222\n", "",
	)
	return replacer.Replace(data)
}

func hasPortInConfig(port int) bool {
	if data, err := os.ReadFile(mainSSHConfig); err == nil {
		if strings.Contains(stripComments(string(data)), fmt.Sprintf("Port %d", port)) {
			return true
		}
	}
	entries, err := os.ReadDir(dropInDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(dropInDir, entry.Name())); err == nil {
			if strings.Contains(stripComments(string(data)), fmt.Sprintf("Port %d", port)) {
				return true
			}
		}
	}
	return false
}

func detectSSHConfig() bool {
	if _, err := os.Stat(dropInFile); err == nil {
		return true
	}
	data, err := os.ReadFile(mainSSHConfig)
	if err != nil {
		return false
	}
	return strings.Contains(stripComments(string(data)), "Port 2222")
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0o644)
}

func parseUFWStatus(output string, port int) bool {
	if strings.Contains(strings.ToLower(output), "status: inactive") {
		return false
	}
	needle := fmt.Sprintf("%d/tcp", port)
	for _, line := range strings.Split(output, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "Status:") || strings.HasPrefix(trim, "To ") {
			continue
		}
		if strings.HasPrefix(trim, needle) || strings.Contains(trim, " "+needle+" ") {
			// 若包含 DENY/REJECT 则视为未放行
			upper := strings.ToUpper(trim)
			if strings.Contains(upper, "DENY") || strings.Contains(upper, "REJECT") {
				return false
			}
			return true
		}
	}
	return false
}

func pidsByPort(ctx context.Context, port int) ([]int, error) {
	if port <= 0 {
		return nil, errors.New("invalid port")
	}
	if commandExists("lsof") {
		cmd := fmt.Sprintf("lsof -t -iTCP:%d -sTCP:LISTEN 2>/dev/null", port)
		stdout, _, err := runCommand(ctx, "sh", "-lc", cmd)
		if err == nil {
			pids := parsePIDList(stdout)
			if len(pids) > 0 {
				return pids, nil
			}
		}
	}
	if commandExists("ss") {
		cmd := fmt.Sprintf("ss -ntlp 2>/dev/null | awk -v P=\":%d \" '$0 ~ P { if (match($NF, /pid=([0-9]+)/, m)) print m[1] }'", port)
		stdout, _, err := runCommand(ctx, "sh", "-lc", cmd)
		if err == nil {
			pids := parsePIDList(stdout)
			if len(pids) > 0 {
				return pids, nil
			}
		}
	}
	return []int{}, nil
}

func parsePIDList(out string) []int {
	fields := strings.Fields(out)
	seen := make(map[int]struct{})
	var pids []int
	for _, f := range fields {
		pid, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || pid <= 1 {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	return pids
}
