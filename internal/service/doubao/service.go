package doubao

import (
	"log/slog"
	"net/http"

	"DoubaoProxy/internal/config"
	"DoubaoProxy/internal/session"
)

// Service 封装与豆包交互的核心业务逻辑，对应原 Python 实现。
type Service struct {
	pool            *session.Pool
	cfg             config.Config
	httpClient      *http.Client
	streamingClient *http.Client
	logger          *slog.Logger
}

// NewService 创建 Service 实例，并配置合适的 HTTP 客户端。
func NewService(pool *session.Pool, cfg config.Config, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	timeout := cfg.HTTPClientTimeout
	stdClient := &http.Client{Timeout: timeout}
	streamClient := &http.Client{Timeout: timeout}

	return &Service{
		pool:            pool,
		cfg:             cfg,
		httpClient:      stdClient,
		streamingClient: streamClient,
		logger:          logger,
	}
}
