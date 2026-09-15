package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type YP struct {
	ChannelsURL string `toml:"channels_url"`
	Name        string `toml:"name"`
	Addr        string `toml:"addr"`
}

// HostPort returns the "host:port" string for use with pcp.Dial.
// Accepts either a plain "host:port" or a "pcp://host[:port]/" URL.
// The default port is 7144.
func (y *YP) HostPort() (string, error) {
	u, err := url.Parse(y.Addr)
	if err != nil || u.Scheme == "" {
		// Treat as plain host:port.
		return y.Addr, nil
	}
	if u.Scheme != "pcp" {
		return "", fmt.Errorf("unsupported scheme %q in yp addr %q", u.Scheme, y.Addr)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "7144"
	}
	return net.JoinHostPort(host, port), nil
}

type Config struct {
	PublicIPv4   string `toml:"public_ipv4"` // Explicit advertised address for NAT/container deployments.
	Site         Site   `toml:"site"`
	RTMPPort     int    `toml:"rtmp_port"`
	PeercastPort int    `toml:"peercast_port"`
	LogLevel     string `toml:"log_level"`
	// MaxRelays はチャンネルに直接接続できる下流リレーノード数の上限。
	// 0 は無制限。
	MaxRelays int `toml:"max_relays"`
	// MaxRelaysTotal は全チャンネル合計のリレー数上限。0 は無制限。
	MaxRelaysTotal int `toml:"max_relays_total"`
	// MaxListeners はチャンネルに直接接続できる HTTP 視聴者数の上限。
	// 0 は無制限。
	MaxListeners int `toml:"max_listeners"`
	// MaxUpstreamKbps は全チャンネル合計の上り帯域上限 (kbps)。0 は無制限。
	MaxUpstreamKbps int `toml:"max_upstream_kbps"`
	// ContentBufferSeconds はコンテンツリングバッファが保持する秒数。
	// ビットレートからパケット数を自動計算する。0 はデフォルト (8秒) を使用。
	ContentBufferSeconds float64 `toml:"content_buffer_seconds"`
	// ChannelCleanupMinutes はリレーチャンネルの自動切断タイムアウト (分)。
	// 視聴・リレーのないリレーチャンネルがこの時間継続すると自動削除される。
	// 0 は自動切断を無効にする。デフォルト 20 分。
	ChannelCleanupMinutes int `toml:"channel_cleanup_minutes"`
	// RelayRequestFrom は未登録チャンネルへの /pls/ /stream/ 要求で
	// オンデマンドリレーを開始できる送信元。"private" (デフォルト) は
	// ループバックとプライベートアドレス (RFC 1918 / fc00::/7 / リンクローカル)
	// のみ、"any" は制限なし。登録済みチャンネルの視聴には影響しない。
	RelayRequestFrom string `toml:"relay_request_from"`
	// MaxRelayChannels はオンデマンドリレーで同時に保持するリレーチャンネル数の
	// 上限。0 は無制限。
	MaxRelayChannels int `toml:"max_relay_channels"`
	// AdminUser / AdminPass は非 localhost からの JSON-RPC アクセスに使う
	// Basic 認証の資格情報。どちらか空の場合は非 localhost を拒否する。
	AdminUser string `toml:"admin_user"`
	AdminPass string `toml:"admin_pass"`
	// AllowedOrigins は JSON-RPC への CORS 要求を許可するオリジンの一覧
	// (例: "https://ui.example.com")。ループバック (localhost / 127.0.0.1 / [::1])
	// のオリジンは常に許可される。それ以外のオリジンからの要求は 403 で拒否する。
	AllowedOrigins []string `toml:"allowed_origins"`
	YPs            []YP     `toml:"yp"`
}

// Site is opt-in; secrets are read from the named process environment variables.
type Site struct {
	BroadcastHistoryDir string   `toml:"broadcast_history_dir"`
	AdminXIDs           []string `toml:"admin_x_ids"`
	MaxRelayChannels    int      `toml:"max_relay_channels"`
	DevLogin            bool     `toml:"dev_login"`
	Enabled             bool     `toml:"enabled"`
	Listen              string   `toml:"listen"`
	Origin              string   `toml:"origin"`
	BasePath            string   `toml:"base_path"`
	UIDir               string   `toml:"ui_dir"`
	RTMPURL             string   `toml:"rtmp_url"`
	MaxViewers          int      `toml:"max_viewers"`
	MaxViewersPerUser   int      `toml:"max_viewers_per_user"`
}

func defaults() Config {
	return Config{
		RTMPPort:              1935,
		PeercastPort:          7144,
		LogLevel:              "info",
		ChannelCleanupMinutes: 20,
		RelayRequestFrom:      "private",
	}
}

// RelayRequestFromAny reports whether any remote may start an on-demand
// relay. Returns an error for an unknown RelayRequestFrom value.
func (c *Config) RelayRequestFromAny() (bool, error) {
	switch strings.ToLower(c.RelayRequestFrom) {
	case "", "private":
		return false, nil
	case "any":
		return true, nil
	default:
		return false, fmt.Errorf("config: relay_request_from must be \"private\" or \"any\", got %q", c.RelayRequestFrom)
	}
}

// SlogLevel converts the LogLevel string to a slog.Level.
// Accepted values: "debug", "info", "warn", "error" (case-insensitive).
// Unknown values fall back to Info.
func (c *Config) SlogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Load reads the TOML config file at path. Missing fields fall back to defaults.
func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if cfg.PublicIPv4 != "" {
		ip := net.ParseIP(cfg.PublicIPv4)
		if ip == nil || ip.To4() == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() {
			return nil, fmt.Errorf("config: public_ipv4 must be a public IPv4 literal")
		}
	}
	return &cfg, nil
}

// FindYP returns the YP entry matching name.
// If name is empty, the first entry is returned.
// Returns an error if the list is empty or name is not found.
func (c *Config) FindYP(name string) (*YP, error) {
	if len(c.YPs) == 0 {
		return nil, fmt.Errorf("config: no yp entries defined")
	}
	if name == "" {
		return &c.YPs[0], nil
	}
	for i := range c.YPs {
		if c.YPs[i].Name == name {
			return &c.YPs[i], nil
		}
	}
	return nil, fmt.Errorf("config: yp %q not found", name)
}
