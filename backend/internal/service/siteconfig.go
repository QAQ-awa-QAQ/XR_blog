package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// 站点内容与布局配置（data/site.json）
//
// 这是站点正文的**唯一配置源**：
//   · 板块的编排（顺序 / 显隐）与每个板块的全部文案都在这里；
//   · 管理后台「站点配置」可视化编辑的也是这份文件；
//   · 管理员也可以直接在服务器上改这个文件（mtime 变化后自动重读），
//     改完刷新页面即生效 —— 永远不需要重建镜像。
//
// 设计边界（刻意的）：
//   · 板块「类型」固定为 4 种渲染器（intro / features / contact / more），
//     每种最多出现一次 —— 类型即稳定键（导航图标、动效也按它映射）；
//   · 样式（CSS）不是配置项；「更多」页的功能按钮（管理后台 / 退出登录）
//     是功能不是内容，同样不是配置项。
// ---------------------------------------------------------------------------

// SiteConfig 是整个站点正文的根配置。
type SiteConfig struct {
	Version  int             `json:"version"`
	Brand    string          `json:"brand"`
	Welcome  WelcomeCopy     `json:"welcome"`
	Sections []SectionConfig `json:"sections"`
}

// WelcomeCopy 欢迎页的标题与副标题。
type WelcomeCopy struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
}

// SectionType 板块渲染器类型。每种类型最多出现一次。
type SectionType string

const (
	SectionIntro    SectionType = "intro"
	SectionFeatures SectionType = "features"
	SectionContact  SectionType = "contact"
	SectionMore     SectionType = "more"
)

// SectionConfig 一个板块（主页一屏 + 侧栏一个导航位）。
type SectionConfig struct {
	Type    SectionType `json:"type"`
	Label   string      `json:"label"` // 侧栏 aria-label / 分页导航文案
	Visible bool        `json:"visible"`
	Eyebrow string      `json:"eyebrow"`
	Title   string      `json:"title"`
	Desc    string      `json:"desc"`

	// 类型专属字段（按 Type 取用其一，其余为 nil）
	Stats    []StatItem    `json:"stats,omitempty"`    // intro
	Items    []ContactItem `json:"items,omitempty"`    // contact
	Settings *MoreSettings `json:"settings,omitempty"` // more
}

// StatItem 简介页的数据卡片。
type StatItem struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ContactItem 联系方式条目。
type ContactItem struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Hint  string `json:"hint"`
	Href  string `json:"href"` // 可空；否则 mailto: 或 http(s)://
}

// MoreSettings 「更多」页设置区的文案。
type MoreSettings struct {
	Follow struct {
		Name string `json:"name"`
		Hint string `json:"hint"`
	} `json:"follow"`
	Slot struct {
		Name       string `json:"name"`
		HintAuto   string `json:"hintAuto"`
		HintManual string `json:"hintManual"`
	} `json:"slot"`
}

// emailPattern 与前端 check-content 的选择一致：非空白、一个 @、域名带点。
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// maxSections 板块数量上限（导航与翻页的可用性边界，不是技术限制）。
const maxSections = 8

// DefaultSiteConfig 内置默认配置（占位文案）：
// data/site.json 不存在时自动落盘这一份，之后一切以文件为准。
// ⚠️ 仓库里只放占位值；真实个人信息（邮箱等）只存在于服务器上的 data/site.json。
func DefaultSiteConfig() *SiteConfig {
	return &SiteConfig{
		Version: 1,
		Brand:   "XR 个人站",
		Welcome: WelcomeCopy{Title: "欢迎访问XR个人站", Subtitle: "一个关于我、我的作品与联系方式的地方"},
		Sections: []SectionConfig{
			{
				Type: SectionIntro, Label: "首页", Visible: true,
				Eyebrow: "Intro", Title: "你好，我是 XR",
				Desc: "做后端与前端之间的事：Go 服务、TypeScript 界面，以及把它们可靠地装进容器里。这个站点既是名片，也是一些自用小工具的入口。",
				Stats: []StatItem{
					{Value: "6", Label: "年工程经验"},
					{Value: "18", Label: "已交付项目"},
					{Value: "Go / TS", Label: "主力技术栈"},
					{Value: "UTC+8", Label: "所在时区"},
				},
			},
			{
				Type: SectionFeatures, Label: "功能", Visible: true,
				Eyebrow: "Features", Title: "功能入口",
				Desc: "以下为占位内容，接口就绪后会替换成真实入口。",
			},
			{
				Type: SectionContact, Label: "联系", Visible: true,
				Eyebrow: "Contact", Title: "联系方式",
				Desc: "本站不提供表单与评论，直接通过下列方式联系即可。",
				Items: []ContactItem{
					{Title: "电子邮箱", Value: "hi@example.com", Hint: "工作日 24 小时内回复", Href: "mailto:hi@example.com"},
					{Title: "代码仓库", Value: "github.com/example", Hint: "开源项目与提交记录", Href: "https://github.com"},
				},
			},
			{
				Type: SectionMore, Label: "更多", Visible: true,
				Eyebrow: "More", Title: "更多",
				Desc:     "本站的基本信息与当前账户。主题色默认跟随本地时间连续过渡，也可以在这里手动指定时段。",
				Settings: defaultMoreSettings(),
			},
		},
	}
}

func defaultMoreSettings() *MoreSettings {
	s := &MoreSettings{}
	s.Follow.Name = "跟随时间"
	s.Follow.Hint = "每分钟按本地时间重新取色，背景连续过渡，不会到点突跳。"
	s.Slot.Name = "主题时段"
	s.Slot.HintAuto = "跟随时间中 · 当前"
	s.Slot.HintManual = "已固定为"
	return s
}

// normalize 清洗：去空白、剔除编辑过程中留下的全空行、清掉与类型无关的字段。
func (s *SiteConfig) normalize() {
	s.Brand = strings.TrimSpace(s.Brand)
	s.Welcome.Title = strings.TrimSpace(s.Welcome.Title)
	s.Welcome.Subtitle = strings.TrimSpace(s.Welcome.Subtitle)

	for i := range s.Sections {
		sec := &s.Sections[i]
		sec.Label = strings.TrimSpace(sec.Label)
		sec.Eyebrow = strings.TrimSpace(sec.Eyebrow)
		sec.Title = strings.TrimSpace(sec.Title)
		sec.Desc = strings.TrimSpace(sec.Desc)

		stats := sec.Stats[:0]
		for _, st := range sec.Stats {
			st.Value = strings.TrimSpace(st.Value)
			st.Label = strings.TrimSpace(st.Label)
			if st.Value == "" && st.Label == "" {
				continue // 后台「添加一行」后没填的行，允许直接去掉
			}
			stats = append(stats, st)
		}
		if len(stats) == 0 {
			stats = nil
		}
		sec.Stats = stats

		items := sec.Items[:0]
		for _, it := range sec.Items {
			it.Title = strings.TrimSpace(it.Title)
			it.Value = strings.TrimSpace(it.Value)
			it.Hint = strings.TrimSpace(it.Hint)
			it.Href = strings.TrimSpace(it.Href)
			if it.Title == "" && it.Value == "" && it.Hint == "" && it.Href == "" {
				continue
			}
			items = append(items, it)
		}
		if len(items) == 0 {
			items = nil
		}
		sec.Items = items

		// 类型专属字段只保留自己那份，避免换类型/复制粘贴带来的脏数据
		if sec.Type != SectionIntro {
			sec.Stats = nil
		}
		if sec.Type != SectionContact {
			sec.Items = nil
		}
		if sec.Type != SectionMore {
			sec.Settings = nil
		}
		if sec.Type == SectionMore && sec.Settings == nil {
			sec.Settings = defaultMoreSettings()
		}
	}
}

// Validate 全量校验（先 normalize）。错误信息面向管理员，直接展示给编辑者。
func (s *SiteConfig) Validate() error {
	s.normalize()

	if s.Version != 1 {
		return errors.New("配置版本不受支持")
	}
	if err := textLen("站点名", s.Brand, 1, 24); err != nil {
		return err
	}
	if err := textLen("欢迎页标题", s.Welcome.Title, 4, 24); err != nil {
		return err
	}
	if err := textLen("欢迎页副标题", s.Welcome.Subtitle, 1, 60); err != nil {
		return err
	}

	if len(s.Sections) == 0 {
		return errors.New("至少需要一个板块")
	}
	if len(s.Sections) > maxSections {
		return fmt.Errorf("板块数量不能超过 %d 个", maxSections)
	}

	seen := map[SectionType]bool{}
	for i := range s.Sections {
		sec := &s.Sections[i]
		name := fmt.Sprintf("第 %d 个板块", i+1)

		switch sec.Type {
		case SectionIntro, SectionFeatures, SectionContact, SectionMore:
		default:
			return fmt.Errorf("%s：类型 %q 不认识（只能是 intro / features / contact / more）", name, sec.Type)
		}
		if seen[sec.Type] {
			return fmt.Errorf("%s：类型 %q 重复（每种类型最多一个）", name, sec.Type)
		}
		seen[sec.Type] = true

		if err := textLen(name+"·导航名", sec.Label, 1, 6); err != nil {
			return err
		}
		if err := textLen(name+"·眉题", sec.Eyebrow, 1, 24); err != nil {
			return err
		}
		if err := textLen(name+"·标题", sec.Title, 1, 24); err != nil {
			return err
		}
		if err := textLen(name+"·描述", sec.Desc, 1, 160); err != nil {
			return err
		}

		switch sec.Type {
		case SectionIntro:
			if len(sec.Stats) == 0 || len(sec.Stats) > 8 {
				return fmt.Errorf("%s：数据卡片需为 1-8 个", name)
			}
			for j, st := range sec.Stats {
				if err := textLen(fmt.Sprintf("%s·数据卡 %d 的值", name, j+1), st.Value, 1, 24); err != nil {
					return err
				}
				if err := textLen(fmt.Sprintf("%s·数据卡 %d 的标签", name, j+1), st.Label, 1, 16); err != nil {
					return err
				}
			}
		case SectionContact:
			if len(sec.Items) == 0 || len(sec.Items) > 8 {
				return fmt.Errorf("%s：联系方式条目需为 1-8 个", name)
			}
			for j, it := range sec.Items {
				if err := textLen(fmt.Sprintf("%s·条目 %d 的标题", name, j+1), it.Title, 1, 12); err != nil {
					return err
				}
				if err := textLen(fmt.Sprintf("%s·条目 %d 的值", name, j+1), it.Value, 1, 64); err != nil {
					return err
				}
				if err := textLen(fmt.Sprintf("%s·条目 %d 的说明", name, j+1), it.Hint, 1, 40); err != nil {
					return err
				}
				if err := validateLinkHref(it.Href); err != nil {
					return fmt.Errorf("%s·条目 %d：%v", name, j+1, err)
				}
			}
		case SectionMore:
			st := sec.Settings
			if st == nil {
				return fmt.Errorf("%s：缺少设置区文案", name)
			}
			for field, v := range map[string]string{
				"跟随时间·名称": st.Follow.Name, "跟随时间·说明": st.Follow.Hint,
				"主题时段·名称": st.Slot.Name, "主题时段·跟随说明": st.Slot.HintAuto,
				"主题时段·固定说明": st.Slot.HintManual,
			} {
				if err := textLen(name+"·"+field, v, 1, 80); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// validateLinkHref 链接（可空）：只放行 http(s):// 与 mailto: ——
// 防 javascript: 之类被浏览器的 a[href] 执行。
func validateLinkHref(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > 512 {
		return errors.New("链接过长（上限 512 字符）")
	}
	if strings.HasPrefix(raw, "mailto:") {
		if !emailPattern.MatchString(strings.TrimPrefix(raw, "mailto:")) {
			return errors.New("邮箱链接格式不正确")
		}
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("链接格式不正确")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("链接必须以 http:// 或 https:// 开头")
	}
	if u.Host == "" {
		return errors.New("链接缺少主机名")
	}
	return nil
}

func textLen(field, v string, min, max int) error {
	n := utf8.RuneCountInString(v)
	if n == 0 {
		return fmt.Errorf("%s不能为空", field)
	}
	if n < min {
		return fmt.Errorf("%s过短（当前 %d 字，至少 %d 字）", field, n, min)
	}
	if n > max {
		return fmt.Errorf("%s过长（当前 %d 字，上限 %d 字）", field, n, max)
	}
	return nil
}

// ---------------------------------------------------------------------------
// SiteConfigStore：文件的读 / 写 / 热加载
// ---------------------------------------------------------------------------

// SiteConfigStore 持有 data/site.json 的缓存与读写。
// Get 会检查文件 mtime/size —— 管理员在服务器上手改了文件，无需重启即生效。
type SiteConfigStore struct {
	mu     sync.Mutex
	path   string
	cached *SiteConfig
	mtime  time.Time
	size   int64
	loaded bool
}

func NewSiteConfigStore(path string) *SiteConfigStore {
	return &SiteConfigStore{path: path}
}

// Get 返回当前有效配置（永不失败：文件缺失/损坏时回退到内置默认并记日志）。
func (s *SiteConfigStore) Get() *SiteConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked()
}

func (s *SiteConfigStore) getLocked() *SiteConfig {
	info, err := os.Stat(s.path)
	if err != nil {
		// 文件不存在（首次启动）或不可访问：
		if !s.loaded {
			def := DefaultSiteConfig()
			s.cached = def
			s.loaded = true
			if saveErr := s.saveLocked(def); saveErr != nil {
				log.Printf("[site] 初始化 %s 失败（将以内存默认值运行）: %v", s.path, saveErr)
			} else {
				log.Printf("[site] 已生成默认站点配置 %s", s.path)
			}
		}
		return s.cached
	}

	if s.loaded && info.ModTime().Equal(s.mtime) && info.Size() == s.size {
		return s.cached
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		log.Printf("[site] 读取 %s 失败，沿用上一份配置: %v", s.path, err)
		return s.cached
	}
	cfg := DefaultSiteConfig()
	if err := json.Unmarshal(raw, cfg); err != nil {
		log.Printf("[site] 解析 %s 失败，沿用上一份配置: %v", s.path, err)
		return s.cached
	}
	if err := cfg.Validate(); err != nil {
		log.Printf("[site] 校验 %s 失败，沿用上一份配置: %v", s.path, err)
		return s.cached
	}

	s.cached = cfg
	s.mtime = info.ModTime()
	s.size = info.Size()
	s.loaded = true
	return s.cached
}

// Update 校验并落盘（原子写：同目录临时文件 + rename），随后刷新缓存。
// 注意：入参会被就地 normalize，调用方可直接使用规范化后的结果。
func (s *SiteConfigStore) Update(cfg *SiteConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveLocked(cfg); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	return nil
}

// saveLocked 原子写并刷新缓存/stat（调用方需持锁）。
func (s *SiteConfigStore) saveLocked(cfg *SiteConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}

	if info, err := os.Stat(s.path); err == nil {
		s.mtime = info.ModTime()
		s.size = info.Size()
	}
	s.cached = cfg
	s.loaded = true
	return nil
}
