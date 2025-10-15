package config

import (
	"os"
	"strconv"
	"time"
)

// Config 描述 HTTP 服务运行时所需的配置项。
type Config struct {
	Addr              string
	SessionConfigPath string
	ShutdownTimeout   time.Duration
	HTTPClientTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
}

// Load 从环境变量加载配置，并在缺省时应用合理的默认值。
//
//	HTTP_ADDR             - HTTP 服务监听地址（默认 :8000）
//	SESSION_CONFIG        - Session 配置 JSON 的路径（默认 session.json）
//	SHUTDOWN_TIMEOUT_SEC  - 优雅关机等待时间，单位秒（默认 10）
//	HTTP_CLIENT_TIMEOUT_S - 上游 HTTP 请求超时时间，单位秒（默认 300）
//	HTTP_READ_TIMEOUT_S   - 服务器读取超时时间，单位秒（默认 30）
//	HTTP_WRITE_TIMEOUT_S  - 服务器写入超时时间，单位秒（默认 30）
func Load() Config {
	return Config{
		Addr:              getenv("HTTP_ADDR", ":8000"),
		SessionConfigPath: getenv("SESSION_CONFIG", "session.json"),
		ShutdownTimeout:   parseDurationSeconds("SHUTDOWN_TIMEOUT_SEC", 10),
		HTTPClientTimeout: parseDurationSeconds("HTTP_CLIENT_TIMEOUT_S", 300),
		ReadTimeout:       parseDurationSeconds("HTTP_READ_TIMEOUT_S", 300),
		WriteTimeout:      parseDurationSeconds("HTTP_WRITE_TIMEOUT_S", 300),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDurationSeconds(key string, fallback int) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return time.Duration(fallback) * time.Second
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(v) * time.Second
}
