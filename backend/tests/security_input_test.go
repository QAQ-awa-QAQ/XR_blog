package tests

// 安全测试 · 输入注入与解析攻面。
//
// 覆盖：
//   - 功能地址的伪协议 / 绕过矩阵（javascript 大小写、控制字符、编码、协议套嵌）
//   - 地址长度边界、白名单图标全量可用（56 个一个不落地验证）与收紧
//   - 标题/描述/标签的长度与空白边界
//   - JSON 类型混淆（数字/对象/数组混入字符串字段）
//   - 存储型 HTML 载荷原样入出（API 层不做破坏性处理）
//   - 拖拽排序与组授权的事务原子性（非法输入必须整体回滚）
//   - 邀请码的参数边界、熵质量与并发消耗（TOCTOU 超发）
//   - 密码哈希验证对敌意参数的抗性（OOM / 除零 / 无限迭代）

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"personal_blog/internal/model"
	"personal_blog/internal/security"
	"personal_blog/internal/service"
)

// 功能地址是下发到 window.open / location 的终极出口：
// 所有伪协议与绕过形态必须全部被拒，否则就是一个 XSS 入口。
func TestSecurityFeatureURLValidationMatrix(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()

	hostile := []string{
		"javascript:alert(1)",
		"JaVaScRiPt:alert(1)",
		"JAVASCRIPT:alert(1)",
		" javascript:alert(1)",  // 前导空格（trim 后仍应识别）
		"\tjavascript:alert(1)", // 前导制表符
		"java\tscript:alert(1)", // 内嵌制表符
		"java\nscript:alert(1)", // 内嵌换行
		"jav\x00ascript:alert(1)",
		"vbscript:msgbox(1)",
		"data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==",
		"file:///etc/passwd",
		"ftp://evil.example/",
		"gopher://evil.example/",
		"//evil.example/",                           // 协议相对
		`\\evil.example\share`,                      // UNC 形态
		"javascript&colon;alert(1)",                 // 实体混淆
		"%6a%61%76%61%73%63%72%69%70%74%3Aalert(1)", // 编码混淆
		"http://",             // 缺主机
		"https://",            // 缺主机
		`http:/\/\ok.example`, // 畸形斜杠
		"not a url at all",
	}
	for _, raw := range hostile {
		_, err := e.Features.Create(ctx, service.FeatureInput{
			Title: "配置", Desc: "地址校验", Tag: "测试", Icon: "server", URL: raw,
		})
		if err == nil {
			t.Fatalf("伪协议/畸形地址 %q 必须被拒绝", raw)
		}
	}

	legit := []string{
		"http://a.example/",
		"https://a.example",
		"http://192.168.11.9:5000/x?y=1#z",
		"HTTP://UPPER.example/", // 协议大小写不敏感
		"https://例子.中国/路径",
	}
	for _, raw := range legit {
		if _, err := e.Features.Create(ctx, service.FeatureInput{
			Title: "配置", Desc: "地址校验", Tag: "测试", Icon: "server", URL: raw,
		}); err != nil {
			t.Fatalf("合法地址 %q 不应被拒: %v", raw, err)
		}
	}
}

// 地址长度边界：512 字符为上限（防超长载荷淹没前端与数据库）。
func TestSecurityFeatureURLBoundary(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()

	prefix := "http://a.example/"
	url512 := prefix + strings.Repeat("p", 512-len(prefix))
	if len(url512) != 512 {
		t.Fatalf("构造失误: %d", len(url512))
	}
	if _, err := e.Features.Create(ctx, service.FeatureInput{
		Title: "边界", Desc: "长度边界", Tag: "测试", Icon: "server", URL: url512,
	}); err != nil {
		t.Fatalf("512 字符地址应接受: %v", err)
	}
	if _, err := e.Features.Create(ctx, service.FeatureInput{
		Title: "边界", Desc: "长度边界", Tag: "测试", Icon: "server", URL: url512 + "p",
	}); err == nil {
		t.Fatal("513 字符地址必须被拒绝")
	}
}

// 图标白名单：56 个值必须全部可用（与前端 featureIcons.ts 镜像），
// 表外任何值（含大小写变体、注入形态）必须全部被拒，且表内无重复项。
func TestSecurityAllWhitelistedIconsUsableAndClosed(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()

	seen := make(map[string]bool, len(service.AllowedIcons))
	for _, icon := range service.AllowedIcons {
		if seen[icon] {
			t.Fatalf("白名单出现重复项 %q", icon)
		}
		seen[icon] = true
		if _, err := e.Features.Create(ctx, service.FeatureInput{
			Title: "图标", Desc: "依次验证", Tag: "测", Icon: icon, URL: "",
		}); err != nil {
			t.Fatalf("白名单图标 %q 不可用: %v", icon, err)
		}
	}
	if len(service.AllowedIcons) != 56 {
		t.Fatalf("图标白名单应为 56 个（与前端镜像），实际 %d——若增删图标请同步更新本断言", len(service.AllowedIcons))
	}

	for _, icon := range []string{"", "Terminal", "TERMINAL", "hacker", "terminal-x", `"><script>`, "🖥", strings.Repeat("a", 40)} {
		if _, err := e.Features.Create(ctx, service.FeatureInput{
			Title: "图标", Desc: "依次验证", Tag: "测", Icon: icon, URL: "",
		}); err == nil {
			t.Fatalf("白名单之外的图标 %q 必须被拒绝", icon)
		}
	}
}

// 展示字段的长度与空白边界：超界、全空白一律拒绝。
func TestSecurityFeatureFieldBounds(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()

	cases := []struct {
		name string
		in   service.FeatureInput
		ok   bool
	}{
		{"标题 1 字", service.FeatureInput{Title: "甲", Desc: "描述", Tag: "测", Icon: "book"}, true},
		{"标题 12 字", service.FeatureInput{Title: strings.Repeat("甲", 12), Desc: "描述", Tag: "测", Icon: "book"}, true},
		{"标题 13 字", service.FeatureInput{Title: strings.Repeat("甲", 13), Desc: "描述", Tag: "测", Icon: "book"}, false},
		{"标题全空白", service.FeatureInput{Title: "   ", Desc: "描述", Tag: "测", Icon: "book"}, false},
		{"描述 40 字", service.FeatureInput{Title: "标题", Desc: strings.Repeat("乙", 40), Tag: "测", Icon: "book"}, true},
		{"描述 41 字", service.FeatureInput{Title: "标题", Desc: strings.Repeat("乙", 41), Tag: "测", Icon: "book"}, false},
		{"标签 6 字", service.FeatureInput{Title: "标题", Desc: "描述", Tag: strings.Repeat("丙", 6), Icon: "book"}, true},
		{"标签 7 字", service.FeatureInput{Title: "标题", Desc: "描述", Tag: strings.Repeat("丙", 7), Icon: "book"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.Features.Create(ctx, tc.in)
			if tc.ok && err != nil {
				t.Fatalf("应通过，实际被拒: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("应被拒绝，实际通过")
			}
		})
	}
}

// JSON 类型混淆：字符串字段收到数字/对象/数组时只能 400，绝不能 500 或静默写入。
func TestSecurityFeatureJSONTypeConfusion(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "198.51.100.110"
	creds := withCredentials(adminCreds(t, e, engine, ip))

	payloads := []any{
		map[string]any{"title": 123, "desc": "d", "tag": "t", "icon": "book"},
		map[string]any{"title": map[string]any{"$ne": nil}, "desc": "d", "tag": "t", "icon": "book"},
		map[string]any{"title": []any{"a"}, "desc": "d", "tag": "t", "icon": "book"},
		map[string]any{"title": "ok", "desc": "d", "tag": "t", "icon": "book", "url": 42},
		[]any{1, 2},
		"just a string",
	}
	for i, p := range payloads {
		rec := call(t, engine, http.MethodPost, "/api/admin/features", ip, p, creds)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("载荷 #%d 应 400，实际 %d：%s", i, rec.Code, rec.Body.String())
		}
	}
}

// 存储型 HTML 载荷：API 层原样存储与返回（转义是前端渲染层的职责），
// 保证既不 500、也不静默改写数据。
func TestSecurityStoredHTMLVerbatim(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	ctx := t.Context()

	const title = `"><svg>`
	const desc = `<b>粗</b>`
	item, err := e.Features.Create(ctx, service.FeatureInput{
		Title: title, Desc: desc, Tag: "<i>", Icon: "book", URL: "",
	})
	if err != nil {
		t.Fatalf("HTML 载荷应被接受（内容原样存储）: %v", err)
	}

	var stored model.Feature
	if err := e.DB.First(&stored, "key = ?", item.Key).Error; err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if stored.Title != title || stored.Desc != desc {
		t.Fatalf("存储内容被意外改写: %+v", stored)
	}

	// 公开接口解码后也必须逐字节一致
	rec := call(t, engine, http.MethodGet, "/api/features", "198.51.100.111", nil, nil)
	for _, f := range decode(t, rec)["features"].([]any) {
		fm := f.(map[string]any)
		if fm["key"] == item.Key {
			if fm["title"] != title || fm["desc"] != desc {
				t.Fatalf("公开列表内容被改写: %v", fm)
			}
			return
		}
	}
	t.Fatal("公开列表未包含刚创建的功能")
}

// 拖拽排序的事务原子性：数组里混入非法 key 时整体回滚，
// 不能出现"前几个已生效、后面失败"的半完成状态。
func TestSecurityReorderAtomicity(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()

	mk := func(name string) *model.Feature {
		item, err := e.Features.Create(ctx, service.FeatureInput{
			Title: name, Desc: "排序测试", Tag: "测", Icon: "book", URL: "",
		})
		if err != nil {
			t.Fatalf("创建 %s 失败: %v", name, err)
		}
		return item
	}
	a, b, c := mk("甲"), mk("乙"), mk("丙")

	order := func() []string {
		list, err := e.Features.List(ctx)
		if err != nil {
			t.Fatalf("列表失败: %v", err)
		}
		var keys []string
		for _, it := range list {
			keys = append(keys, it.Key)
		}
		return keys
	}
	if got := order(); len(got) != 3 || got[0] != a.Key || got[1] != b.Key || got[2] != c.Key {
		t.Fatalf("初始顺序异常: %v", got)
	}

	// 含非法 key：整体拒绝，顺序纹丝不动
	if err := e.Features.Reorder(ctx, []string{c.Key, b.Key, "BAD KEY!"}); err == nil {
		t.Fatal("含非法 key 的排序必须被拒绝")
	}
	if got := order(); got[0] != a.Key || got[1] != b.Key || got[2] != c.Key {
		t.Fatalf("失败的排序不得留下部分修改，实际顺序 %v", got)
	}

	// 合法排序生效
	if err := e.Features.Reorder(ctx, []string{c.Key, b.Key, a.Key}); err != nil {
		t.Fatalf("合法排序失败: %v", err)
	}
	if got := order(); got[0] != c.Key || got[1] != b.Key || got[2] != a.Key {
		t.Fatalf("排序结果不符: %v", got)
	}

	// 大量不存在但格式合法的 key：只应更新 0 行，不得破坏既有顺序
	ghosts := make([]string, 200)
	for i := range ghosts {
		ghosts[i] = fmt.Sprintf("ghost-%03d", i)
	}
	if err := e.Features.Reorder(ctx, ghosts); err != nil {
		t.Fatalf("不存在的 key 应被宽容处理: %v", err)
	}
	if got := order(); got[0] != c.Key || got[1] != b.Key || got[2] != a.Key {
		t.Fatalf("幽灵排序污染了真实顺序: %v", got)
	}
}

// 组授权的覆盖式更新同样是事务：非法授权集合必须整体回滚，
// 绝不能"旧的被清空、新的没写全"。
func TestSecurityGroupGrantAtomicity(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()

	u, err := e.Auth.Register(ctx, registerInput("grant_user", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	g, err := e.Access.CreateGroup(ctx, "授权原子组")
	if err != nil {
		t.Fatalf("建组失败: %v", err)
	}
	initial := []string{"nas"}
	if err := e.Access.UpdateGroup(ctx, g.ID, nil, &initial); err != nil {
		t.Fatalf("授权失败: %v", err)
	}
	if err := e.Access.SetMembers(ctx, g.ID, []uint{u.ID}); err != nil {
		t.Fatalf("设置成员失败: %v", err)
	}
	if ok, _ := e.Access.CanOpen(ctx, u.ID, "nas"); !ok {
		t.Fatal("前置条件失败: nas 应已授权")
	}

	// 非法 key 混合在合法集合里：整体回滚
	bad := []string{"grafana", "BAD!!"}
	if err := e.Access.UpdateGroup(ctx, g.ID, nil, &bad); err == nil {
		t.Fatal("含非法 key 的授权必须被拒绝")
	}
	if ok, _ := e.Access.CanOpen(ctx, u.ID, "nas"); !ok {
		t.Fatal("失败的授权更新不得清空旧授权（nas 应仍有效）")
	}
	if ok, _ := e.Access.CanOpen(ctx, u.ID, "grafana"); ok {
		t.Fatal("失败的授权更新不得写入新授权（grafana 不应生效）")
	}

	// 重复项触发主键冲突：同样整体回滚
	dup := []string{"nas", "nas"}
	if err := e.Access.UpdateGroup(ctx, g.ID, nil, &dup); err == nil {
		t.Fatal("含重复项的授权必须被拒绝")
	}
	if ok, _ := e.Access.CanOpen(ctx, u.ID, "nas"); !ok {
		t.Fatal("主键冲突回滚后旧授权必须保留")
	}
}

// 邀请码参数边界：次数 1-100、天数 1-365，类型混淆与数字溢出
// 无论落到哪个分支，都绝不能创建超出上限的码。
func TestSecurityInviteBoundsHTTP(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "198.51.100.112"
	creds := withCredentials(adminCreds(t, e, engine, ip))

	cases := []struct {
		name     string
		body     map[string]any
		wantCode int // 0 = 不限定状态码，只验证"不超上限"
	}{
		{"空体走默认", nil, http.StatusOK},
		{"0 次走默认", map[string]any{"maxUses": 0}, http.StatusOK},
		{"负数次数拒绝", map[string]any{"maxUses": -1}, http.StatusBadRequest},
		{"101 次拒绝", map[string]any{"maxUses": 101}, http.StatusBadRequest},
		{"100 次上限", map[string]any{"maxUses": 100}, http.StatusOK},
		{"负数天数拒绝", map[string]any{"expiresInDays": -1}, http.StatusBadRequest},
		{"366 天拒绝", map[string]any{"expiresInDays": 366}, http.StatusBadRequest},
		{"365 天上限", map[string]any{"expiresInDays": 365}, http.StatusOK},
		{"次数类型混淆", map[string]any{"maxUses": "abc"}, 0},
		{"次数数字溢出", map[string]any{"maxUses": float64(99999999999999999999)}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := call(t, engine, http.MethodPost, "/api/admin/invites", ip, tc.body, creds)
			if tc.wantCode != 0 {
				if rec.Code != tc.wantCode {
					t.Fatalf("预期 %d，实际 %d：%s", tc.wantCode, rec.Code, rec.Body.String())
				}
			} else if rec.Code >= 500 {
				t.Fatalf("类型混淆不应 5xx，实际 %d", rec.Code)
			}
			if rec.Code == http.StatusOK {
				if max, ok := decode(t, rec)["maxUses"].(float64); !ok || max > 100 || max < 1 {
					t.Fatalf("无论输入如何，绝不允许创建超出 1-100 的码，实际 %v", decode(t, rec)["maxUses"])
				}
			}
		})
	}
}

// 邀请码必须是高强度随机：16 位、字符集固定、样本内零重复。
func TestSecurityInviteCodeEntropy(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()

	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	seen := make(map[string]bool, 64)
	for i := 0; i < 64; i++ {
		inv, err := e.Invite.Create(ctx, 1, 1, time.Hour)
		if err != nil {
			t.Fatalf("生成失败: %v", err)
		}
		if len(inv.Code) != 16 {
			t.Fatalf("邀请码应为 16 位，实际 %d（%s）", len(inv.Code), inv.Code)
		}
		for _, r := range inv.Code {
			if !strings.ContainsRune(alphabet, r) {
				t.Fatalf("邀请码含字符集外字符 %q：%s", r, inv.Code)
			}
		}
		if seen[inv.Code] {
			t.Fatalf("邀请码重复：%s", inv.Code)
		}
		seen[inv.Code] = true
	}
}

// 并发消耗同一张一次性邀请码：恰好一人成功（原子占位，无 TOCTOU 超发）。
func TestSecurityConcurrentInviteConsumption(t *testing.T) {
	e := newEnv(t)

	invite, err := e.Invite.Create(t.Context(), 1, 1, time.Hour)
	if err != nil {
		t.Fatalf("生成邀请码失败: %v", err)
	}

	names := []string{"race_a", "race_b"}
	errs := make([]error, len(names))
	var wg sync.WaitGroup
	for i := range names {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = e.Auth.Register(t.Context(), registerInput(names[i], invite.Code))
		}(i)
	}
	wg.Wait()

	success := 0
	for i, err := range errs {
		switch {
		case err == nil:
			success++
		case errors.Is(err, service.ErrInviteInvalid):
			// 预期中的失败分支
		default:
			t.Fatalf("并发注册 #%d 返回了非预期错误: %v", i, err)
		}
	}
	if success != 1 {
		t.Fatalf("一次性邀请码并发注册应恰好 1 人成功，实际 %d 人", success)
	}

	var n int64
	if err := e.DB.Model(&model.User{}).Where("account IN ?", names).Count(&n).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("应恰好创建 1 个账号，实际 %d", n)
	}
}

// 密码验证对敌意哈希参数的抗性：超大内存/迭代/并行度、零值、超长盐与哈希
// 都必须立即 false——既不能 OOM/panic，也不能被拖死。
func TestSecurityVerifyPasswordHostileParams(t *testing.T) {
	salt := base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef"))
	key := base64.RawStdEncoding.EncodeToString([]byte("hashhashhashhash"))
	long := base64.StdEncoding.EncodeToString(make([]byte, 1024))

	hostile := []string{
		"$argon2id$v=19$m=4294967295,t=1,p=4$" + salt + "$" + key,     // 4TB 内存
		"$argon2id$v=19$m=1048576,t=1,p=1$" + salt + "$" + key,        // 1GB 内存
		"$argon2id$v=19$m=65536,t=4294967295,p=1$" + salt + "$" + key, // 无限迭代
		"$argon2id$v=19$m=65536,t=1,p=0$" + salt + "$" + key,          // 除零
		"$argon2id$v=19$m=0,t=1,p=1$" + salt + "$" + key,              // 零内存
		"$argon2id$v=19$m=65536,t=0,p=1$" + salt + "$" + key,          // 零迭代
		"$argon2id$v=19$m=99999999999999,t=1,p=1$" + salt + "$" + key, // 解析溢出
		"$argon2id$v=19$m=-1,t=1,p=4$" + salt + "$" + key,             // 负数
		"$argon2id$v=19$m=65536,t=1,p=4$" + long + "$" + key,          // 超长盐
		"$argon2id$v=19$m=65536,t=1,p=4$" + salt + "$" + long,         // 超长哈希
		"$argon2id$v=19$m=65536,t=1,p=4$$" + key,                      // 空盐
	}
	for i, h := range hostile {
		start := time.Now()
		if security.VerifyPassword("whatever", h) {
			t.Fatalf("敌意哈希 #%d 不应通过校验", i)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("敌意哈希 #%d 必须被立即拒绝，实际耗时 %v", i, d)
		}
	}

	// 边界防护不得误杀自产哈希
	good, err := security.HashPassword("boundary-check")
	if err != nil {
		t.Fatalf("生成哈希失败: %v", err)
	}
	if !security.VerifyPassword("boundary-check", good) {
		t.Fatal("正常参数的哈希必须仍能通过（防护误杀）")
	}
	if security.VerifyPassword("wrong-password-x", good) {
		t.Fatal("错误密码不应通过")
	}
}
