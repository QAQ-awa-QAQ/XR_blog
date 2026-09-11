package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 全部来自环境变量并带可用默认值，本机直跑与容器部署共用同一套代码。
type Config struct {
	Addr           string // Go 监听地址；容器内为 127.0.0.1:8081，仅由 Nginx 反代
	DBPath         string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	SessionTTL     time.Duration
	CookieSecure   bool
	TrustedProxies []string
	AdminAccount   string
	AdminPassword  string // 为空则冷启动时随机生成并打印到日志
	InviteTTL      time.Duration

	// design.md 第 4 章安全规则的可调参数
	CooldownWindow   time.Duration
	CooldownLimit    int
	CooldownDuration time.Duration
	BanWindow        time.Duration
	BanLimit         int
	BanDuration      time.Duration
}

func Load() Config {
	return Config{
		Addr:           env("ADDR", ":8081"),
		DBPath:         env("DB_PATH", "data/blog.db"),
		RedisAddr:      env("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:  env("REDIS_PASSWORD", ""),
		RedisDB:        envInt("REDIS_DB", 0),
		SessionTTL:     envDur("SESSION_TTL", 7*24*time.Hour),
		CookieSecure:   envBool("COOKIE_SECURE", true),
		TrustedProxies: envList("TRUSTED_PROXIES", []string{"127.0.0.1", "::1"}),
		AdminAccount:   env("ADMIN_ACCOUNT", "admin"),
		AdminPassword:  env("ADMIN_PASSWORD", ""),
		InviteTTL:      envDur("INVITE_TTL", 7*24*time.Hour),

		CooldownWindow:   envDur("COOLDOWN_WINDOW", 10*time.Second),
		CooldownLimit:    envInt("COOLDOWN_LIMIT", 3),
		CooldownDuration: envDur("COOLDOWN_DURATION", 10*time.Second),
		BanWindow:        envDur("BAN_WINDOW", 20*time.Second),
		BanLimit:         envInt("BAN_LIMIT", 20),
		BanDuration:      envDur("BAN_DURATION", 24*time.Hour),
	}
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return def
}

func envList(key string, def []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
