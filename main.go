package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"DoubaoProxy/internal/config"
	"DoubaoProxy/internal/handler"
	"DoubaoProxy/internal/server"
	"DoubaoProxy/internal/service/doubao"
	"DoubaoProxy/internal/session"
)

func main() {
	cfg := config.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	pool, err := session.NewPool(cfg.SessionConfigPath)
	if err != nil {
		logger.Error("failed to load session pool", "error", err)
		os.Exit(1)
	}

	service := doubao.NewService(pool, cfg, logger)

	srv := server.New(cfg, logger, func(r *gin.Engine) {
		handler.Register(r, service, cfg.AuthToken)
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}
