package proxyurl

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Parse validates and normalizes a proxy URL so that "validation passed" implies
// runtime code will not fail due to the proxy URL itself.
//
// Supported schemes:
// - http, https (host required; port optional)
// - socks5, socks5h (host:port required)
//
// It rejects non-empty query/fragment and non-root paths.
func Parse(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("proxy url is required")
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, err
	}

	u.Scheme = strings.ToLower(strings.TrimSpace(u.Scheme))
	if u.Scheme == "" {
		return nil, fmt.Errorf("proxy scheme is required")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", u.Scheme)
	}

	if strings.TrimSpace(u.Host) == "" || strings.TrimSpace(u.Hostname()) == "" {
		return nil, fmt.Errorf("proxy host is required")
	}
	if strings.HasSuffix(strings.TrimSpace(u.Host), ":") {
		return nil, fmt.Errorf("proxy port is invalid")
	}

	if u.RawQuery != "" {
		return nil, fmt.Errorf("proxy query is not supported")
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("proxy fragment is not supported")
	}
	if u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("proxy path is not supported")
	}
	if u.Path == "/" {
		// Canonicalize a trailing slash.
		u.Path = ""
	}

	portStr := strings.TrimSpace(u.Port())
	if u.Scheme == "socks5" || u.Scheme == "socks5h" {
		if portStr == "" {
			return nil, fmt.Errorf("proxy port is required")
		}
		if _, _, err := net.SplitHostPort(u.Host); err != nil {
			return nil, fmt.Errorf("proxy host must be host:port")
		}
	}
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("proxy port is invalid")
		}
	}

	return u, nil
}

func Normalize(raw string) (string, error) {
	u, err := Parse(raw)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

