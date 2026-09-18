package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"personal_blog/internal/config"
	"personal_blog/internal/handler"
	"personal_blog/internal/router"
	"personal_blog/internal/service"
	"personal_blog/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[blog] ")

	cfg := config.Load()
	ctx := context.Background()

	db, err := store.OpenSQLite(cfg.DBPath)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	rdb, err := store.OpenRedis(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Fatalf("初始化 Redis 失败: %v", err)
	}
	defer rdb.Close()

	sessions := service.NewSessions(rdb, cfg.SessionTTL)
	auth := service.NewAuth(db, sessions)
	invite := service.NewInvite(db, cfg.InviteTTL)
	guard := service.NewGuard(rdb, db, cfg)
	access := service.NewAccess(db)
	features := service.NewFeatures(db)

	generated, created, err := auth.EnsureAdmin(ctx, cfg.AdminAccount, cfg.AdminPassword)
	if err != nil {
		log.Fatalf("初始化管理员账号失败: %v", err)
	}
	if created {
		if generated != "" {
			log.Printf("已创建管理员 %s，初始密码（仅本次显示）：%s", cfg.AdminAccount, generated)
		} else {
			log.Printf("已按 ADMIN_PASSWORD 创建管理员 %s", cfg.AdminAccount)
		}
	}

	// 首次启动播种功能入口（之后一切由管理后台维护）
	if err := features.EnsureDefaults(ctx); err != nil {
		log.Fatalf("初始化功能入口失败: %v", err)
	}

	engine, err := router.New(router.Deps{
		Config:         cfg,
		Auth:           auth,
		Sessions:       sessions,
		Guard:          guard,
		Access:         access,
		Features:       features,
		AuthHandler:    handler.NewAuthHandler(auth, sessions, cfg),
		AdminHandler:   handler.NewAdminHandler(invite, guard, auth),
		AccessHandler:  handler.NewAccessAdminHandler(access),
		FeatureHandler: handler.NewFeatureAdminHandler(features),
	})
	if err != nil {
		log.Fatalf("初始化路由失败: %v", err)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("服务启动于 %s（可信代理：%v）", cfg.Addr, cfg.TrustedProxies)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务异常退出: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，开始优雅关闭…")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("关闭超时: %v", err)
	}
	log.Println("已退出")
}
