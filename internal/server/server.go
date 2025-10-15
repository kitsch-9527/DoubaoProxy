package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"DoubaoProxy/internal/config"
)

// Server 负责 HTTP 服务的生命周期管理。
type Server struct {
	cfg    config.Config
	router *gin.Engine
	logger *slog.Logger
}

// New 创建 Server 并允许调用方注册路由。
func New(cfg config.Config, logger *slog.Logger, register func(*gin.Engine)) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), ginLogger(logger))

	if register != nil {
		register(router)
	}

	return &Server{cfg: cfg, router: router, logger: logger}
}

// Run 启动 HTTP 服务，直到外部上下文被取消才退出。
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:         s.cfg.Addr,
		Handler:      s.router,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func ginLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		logger.Info("http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", latency.String(),
			"ip", c.ClientIP(),
		)
	}
}
