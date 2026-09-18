package store

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"personal_blog/internal/model"
)

// OpenSQLite 打开 SQLite（WAL + busy_timeout），并完成建表。
// 使用纯 Go driver，构建无需 CGO。
func OpenSQLite(path string) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录失败: %w", err)
		}
	}

	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.New(log.New(os.Stdout, "[gorm] ", log.LstdFlags), logger.Config{
			SlowThreshold:             500 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true, // 记录不存在是正常分支，不算错误
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(8)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := db.AutoMigrate(
		&model.User{},
		&model.InviteCode{},
		&model.BanRecord{},
		&model.Group{},
		&model.GroupFeature{},
		&model.UserGroup{},
		&model.Feature{},
	); err != nil {
		return nil, fmt.Errorf("建表失败: %w", err)
	}

	// 兼容旧数据：邀请码从「一次性」升级为「可多次」——历史已用过的折算为 1 次
	if err := db.Exec("UPDATE invite_codes SET used_count = 1 WHERE used_at IS NOT NULL AND used_count = 0").Error; err != nil {
		return nil, fmt.Errorf("回填邀请码用量失败: %w", err)
	}
	return db, nil
}

// OpenRedis 连接 Redis 并做一次探活；限流计数、封禁缓存与登录会话都依赖它。
func OpenRedis(addr, password string, dbIndex int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       dbIndex,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("连接 Redis(%s) 失败: %w", addr, err)
	}
	return client, nil
}
