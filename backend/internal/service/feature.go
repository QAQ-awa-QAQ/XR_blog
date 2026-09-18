package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"personal_blog/internal/model"
)

// Features 功能入口域：展示字段（标题/描述/标签/图标）与内网地址统一入库管理。
// 公开列表只含展示字段；URL 仅在通过点击鉴权后按需下发，永不进入前端产物。
type Features struct{ db *gorm.DB }

func NewFeatures(db *gorm.DB) *Features { return &Features{db: db} }

// ErrFeatureNotConfigured 功能地址尚未配置（功能未上线）。
var ErrFeatureNotConfigured = errors.New("该功能尚未配置地址")

// AllowedIcons 图标白名单（与前端 design/featureIcons.ts 的 56 个键保持一致，
// 两边必须同改）。
var AllowedIcons = []string{
	// 现有 6 个
	"terminal", "chart", "cloud", "shield", "book", "wrench",
	// 服务器与数据
	"server", "database", "hard-drive", "cpu", "activity",
	// 网络
	"globe", "wifi", "router",
	// 文件
	"folder", "file-text", "download", "upload",
	// 开发
	"code", "git",
	// 安全
	"lock", "key", "eye",
	// 媒体
	"image", "camera", "music", "video", "mic", "headphones",
	// 设备
	"monitor", "smartphone", "laptop", "printer",
	// 沟通
	"mail", "message", "phone", "bell",
	// 时间与位置
	"calendar", "clock", "map-pin", "compass", "home",
	// 人与组织
	"user", "users", "tag",
	// 对象与工具
	"layers", "box", "briefcase", "sliders", "zap",
	// 指标
	"pie-chart", "trending-up", "target", "gauge",
	// 气候
	"sun", "moon",
}

func validIcon(icon string) bool {
	for _, name := range AllowedIcons {
		if name == icon {
			return true
		}
	}
	return false
}

// FeatureInput 新建/编辑功能的输入。
type FeatureInput struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
	Tag   string `json:"tag"`
	Icon  string `json:"icon"`
	URL   string `json:"url"`
}

// normalizeAndValidate 清洗并校验展示字段与地址（地址可为空 = 未上线）。
func (in *FeatureInput) normalizeAndValidate() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Desc = strings.TrimSpace(in.Desc)
	in.Tag = strings.TrimSpace(in.Tag)
	in.Icon = strings.TrimSpace(in.Icon)
	in.URL = strings.TrimSpace(in.URL)

	if n := len([]rune(in.Title)); n < 1 || n > 12 {
		return errors.New("标题需为 1-12 个字符")
	}
	if n := len([]rune(in.Desc)); n < 1 || n > 40 {
		return errors.New("描述需为 1-40 个字符")
	}
	if n := len([]rune(in.Tag)); n < 1 || n > 6 {
		return errors.New("标签需为 1-6 个字符")
	}
	if !validIcon(in.Icon) {
		return errors.New("图标不在允许列表内")
	}
	return validateFeatureURL(in.URL)
}

// validateFeatureURL 只允许 http(s)（空串 = 未配置）——
// 防 javascript: 之类被浏览器的 window.open 执行。
func validateFeatureURL(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > 512 {
		return errors.New("地址过长（上限 512 字符）")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("地址格式不正确")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("地址必须以 http:// 或 https:// 开头")
	}
	if u.Host == "" {
		return errors.New("地址缺少主机名")
	}
	return nil
}

// List 公开列表：仅展示字段，按 Sort 排序。查询层面就不带 URL 用途的泄露面。
func (f *Features) List(ctx context.Context) ([]model.Feature, error) {
	var out []model.Feature
	err := f.db.WithContext(ctx).Order("sort ASC, created_at ASC").Find(&out).Error
	return out, err
}

// Get 取完整记录（含 URL）。
func (f *Features) Get(ctx context.Context, key string) (*model.Feature, error) {
	var item model.Feature
	if err := f.db.WithContext(ctx).First(&item, "key = ?", key).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// GetURL 读内网地址；未配置返回 ErrFeatureNotConfigured。
func (f *Features) GetURL(ctx context.Context, key string) (string, error) {
	item, err := f.Get(ctx, key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrFeatureNotConfigured
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(item.URL) == "" {
		return "", ErrFeatureNotConfigured
	}
	return item.URL, nil
}

// Create 新建功能：生成随机 key（授权与地址都以它为准，之后不可改）。
func (f *Features) Create(ctx context.Context, in FeatureInput) (*model.Feature, error) {
	if err := in.normalizeAndValidate(); err != nil {
		return nil, err
	}

	var maxSort int
	if err := f.db.WithContext(ctx).Model(&model.Feature{}).
		Select("COALESCE(MAX(sort), -1)").Scan(&maxSort).Error; err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 5; attempt++ {
		key, err := randomCode(10)
		if err != nil {
			return nil, err
		}
		item := model.Feature{
			Key:   key,
			Title: in.Title,
			Desc:  in.Desc,
			Tag:   in.Tag,
			Icon:  in.Icon,
			URL:   in.URL,
			Sort:  maxSort + 1,
		}
		err = f.db.WithContext(ctx).Create(&item).Error
		if err == nil {
			return &item, nil
		}
		if !isUniqueViolation(err) {
			return nil, err
		}
	}
	return nil, errors.New("生成功能标识失败，请重试")
}

// Update 覆盖式更新展示字段与地址。
func (f *Features) Update(ctx context.Context, key string, in FeatureInput) error {
	if err := in.normalizeAndValidate(); err != nil {
		return err
	}

	res := f.db.WithContext(ctx).Model(&model.Feature{}).Where("key = ?", key).Updates(map[string]any{
		"title": in.Title,
		"desc":  in.Desc,
		"tag":   in.Tag,
		"icon":  in.Icon,
		"url":   in.URL,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 值完全没变时 RowsAffected 也会是 0，先用存在性排除「不存在」
		var count int64
		if err := f.db.WithContext(ctx).Model(&model.Feature{}).Where("key = ?", key).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}

// Delete 删除功能，并清理所有用户组对它的授权。
func (f *Features) Delete(ctx context.Context, key string) error {
	return f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&model.Feature{}, "key = ?", key)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Delete(&model.GroupFeature{}, "feature_key = ?", key).Error
	})
}

// Reorder 按给定顺序重排（数组下标即新 Sort）。
func (f *Features) Reorder(ctx context.Context, keys []string) error {
	return f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, key := range keys {
			if !ValidFeatureKey(key) {
				return errors.New("非法的功能标识：" + key)
			}
			if err := tx.Model(&model.Feature{}).Where("key = ?", key).Update("sort", i).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// EnsureDefaults 首次启动播种：功能表为空时插入初始功能（迁移自旧前端配置）。
// 之后一切改动都通过管理后台，代码不再携带功能文案。
func (f *Features) EnsureDefaults(ctx context.Context) error {
	var count int64
	if err := f.db.WithContext(ctx).Model(&model.Feature{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	defaults := []model.Feature{
		{Key: "terminal", Title: "在线终端", Desc: "把常用脚本收进浏览器，随时执行。", Tag: "开发", Icon: "terminal", Sort: 0},
		{Key: "chart", Title: "数据看板", Desc: "把散落的指标汇总成一张图。", Tag: "分析", Icon: "chart", Sort: 1},
		{Key: "cloud", Title: "资源托管", Desc: "静态资源与文件的分发入口。", Tag: "基建", Icon: "cloud", Sort: 2},
		{Key: "shield", Title: "安全工具", Desc: "限流、封禁与访问审计。", Tag: "安全", Icon: "shield", Sort: 3},
		{Key: "book", Title: "笔记归档", Desc: "长期沉淀的技术笔记索引。", Tag: "内容", Icon: "book", Sort: 4},
		{Key: "wrench", Title: "实验工坊", Desc: "还没定型的小玩意都放这儿。", Tag: "实验", Icon: "wrench", Sort: 5},
	}
	return f.db.WithContext(ctx).Create(&defaults).Error
}
