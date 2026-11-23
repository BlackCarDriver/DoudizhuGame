package main

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/BlackCarDriver/nichijouSDK"
)

const (
	defaultConfigPath      = "config/plugin_config.json"
	defaultIntegrationPath = "config/integration.json"
)

// fileConfig 运行配置：只放本服务自身的部署参数。
type fileConfig struct {
	Addr               string   `json:"addr"`
	WebRoot            string   `json:"webRoot"`
	InstanceID         string   `json:"instanceId"`
	PublicKeyPath      string   `json:"pubkey"`
	CountdownSeconds   int      `json:"countdownSeconds"`
	BidSeconds         int      `json:"bidSeconds"`
	DoubleSeconds      int      `json:"doubleSeconds"`
	TurnSeconds        int      `json:"turnSeconds"`
	SeatGraceSeconds   int      `json:"seatGraceSeconds"`
	AllowSpectatorHand *bool    `json:"allowSpectatorHands"`
	InitialPoints      *int     `json:"initialPoints"`
	MinPointsToSit     *int     `json:"minPointsToSit"`
	DailyActiveReward  *int     `json:"dailyActiveReward"`
	RakePercent        *int     `json:"rakePercent"`
	RakeMinProfit      *int     `json:"rakeMinProfit"`
	RakeCap            *int     `json:"rakeCap"`
	BaseScore          int      `json:"baseScore"`
	DataDir            string   `json:"dataDir"`
	BoardLimit         int      `json:"boardLimit"`
	AdminNicks         []string `json:"adminNicks"`
}

func (f fileConfig) isZero() bool {
	return f.Addr == "" && f.WebRoot == "" && f.InstanceID == "" && f.PublicKeyPath == "" &&
		f.CountdownSeconds == 0 && f.BidSeconds == 0 && f.DoubleSeconds == 0 && f.TurnSeconds == 0 &&
		f.SeatGraceSeconds == 0 && f.AllowSpectatorHand == nil && f.InitialPoints == nil &&
		f.MinPointsToSit == nil && f.DailyActiveReward == nil && f.RakePercent == nil &&
		f.RakeMinProfit == nil && f.RakeCap == nil && f.BaseScore == 0 && f.DataDir == "" && f.BoardLimit == 0 &&
		len(f.AdminNicks) == 0
}

// Config 运行期完整配置。
type Config struct {
	Addr             string
	WebRoot          string
	ChatroomBase     string
	ChatroomWSBase   string
	RoomID           int
	IntegrationID    string
	BridgeSecret     string
	ExchangeSecret   string
	SecretVersion    int64
	InstanceID       string
	PublicKeyPath    string
	PublicKey        *rsa.PublicKey
	CountdownSeconds int
	BidSeconds       int
	DoubleSeconds    int
	TurnSeconds      int
	SeatGraceSeconds int
	// AllowSpectatorHands 房间是否允许观众看牌（默认开）
	AllowSpectatorHands bool
	InitialPoints       int
	MinPointsToSit      int
	DailyActiveReward   int
	RakePercent         int
	RakeMinProfit       int
	RakeCap             int
	BaseScore           int
	DataDir             string
	BoardLimit          int
	AdminNicks          []string
}

func loadFileConfig(path string) (fileConfig, error) {
	var cfg fileConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if len(raw) == 0 {
		return cfg, nil
	}
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func loadIntegration(path string) (nichijouSDK.IntegrationConfig, error) {
	return nichijouSDK.LoadIntegrationConfig(path)
}

func pickString(fileVal, envKey, def string) string {
	if fileVal != "" {
		def = fileVal
	}
	if v := os.Getenv(envKey); v != "" {
		def = v
	}
	return def
}

func pickInt(fileVal int, envKey string, def int) int {
	if fileVal != 0 {
		def = fileVal
	}
	if v := os.Getenv(envKey); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func pickInt64(fileVal int64, envKey string, def int64) int64 {
	if fileVal != 0 {
		def = fileVal
	}
	if v := os.Getenv(envKey); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func pickIntPtr(fileVal *int, envKey string, def int) int {
	if fileVal != nil {
		def = *fileVal
	}
	if v := os.Getenv(envKey); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func pickBool(fileVal *bool, envKey string, def bool) bool {
	if fileVal != nil {
		def = *fileVal
	}
	if v := os.Getenv(envKey); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func pickList(fileVal []string, envKey string, def []string) []string {
	out := def
	if len(fileVal) > 0 {
		out = fileVal
	}
	if v := os.Getenv(envKey); v != "" {
		out = splitList(v)
	}
	return out
}

func chatroomWSBase(base string) string {
	switch {
	case len(base) >= 8 && base[:8] == "https://":
		return "wss://" + base[8:]
	case len(base) >= 7 && base[:7] == "http://":
		return "ws://" + base[7:]
	}
	return base
}

func (c *Config) bridgeReady() bool {
	return c.RoomID != 0 && c.IntegrationID != "" && c.BridgeSecret != "" && c.PublicKey != nil
}

func (c *Config) missingBridgeFields() []string {
	var out []string
	if c.RoomID == 0 {
		out = append(out, "-room-id")
	}
	if c.IntegrationID == "" {
		out = append(out, "-integration-id")
	}
	if c.BridgeSecret == "" {
		out = append(out, "-bridge-secret")
	}
	if c.PublicKey == nil {
		out = append(out, "公钥（-pubkey）")
	}
	return out
}

func (c *Config) String() string {
	return fmt.Sprintf("addr=%s web=%s room=%d integration=%s", c.Addr, c.WebRoot, c.RoomID, c.IntegrationID)
}
