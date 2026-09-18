# 实施方案（已获用户逐条批准）

> 本文件是 `design.md` 的执行细则。两者冲突时以 `design.md` 为准。
> **数值依据、被否决的方案与授权边界见 `DESIGN-LOG.md`。**

## 1. 技术栈

| 层 | 选型 |
|---|---|
| 前端 | React 19 + Vite + TypeScript + GSAP（无 react-router，切换用状态） |
| 后端 | Go + Gin + GORM + SQLite（纯 Go driver `glebarez/sqlite`，免 CGO） |
| 缓存/会话 | Redis 7（AOF 持久化） |
| 部署 | 单容器内含 Nginx + Go 双进程，多阶段构建 |
| 设计 | skill `ui-ux-pro-max` → Liquid Glass 风格，Inter 字体 |

## 2. 安全规则参数（`design.md` 第 4 章的可执行定义）

| 规则 | 触发 | 动作 |
|---|---|---|
| 冷却 | 同 IP 10s 内 ≥3 次登录/注册 | 返回 **429 + 仍计数**，冷却 10s |
| 封禁 L1 | 同 IP 20s 内 ≥20 次 | 封禁 **24h**，`offense_count` +1 |
| 封禁 L2 | 再次触发 L1 | **永久**封禁 |
| 已封禁 | 任意请求（含静态资源） | Nginx 重定向到封禁页 |
| 优先级 | 已封禁 > 冷却 | 封禁期内不再计算冷却 |

- 计数窗口用 Redis `ZSET`（滑动窗口，精确到毫秒），键带 IP 前缀并按窗口大小自动过期。
- `offense_count` 落 SQLite 且**永不清除**——否则重启后无法区分首次/再犯。
- 24h 到期判定用 DB `expires_at` 字段比较时间，无需定时任务。
- 4.3 豁免：`/api/auth/*` 与 admin 专属写接口，详见 §5 白名单。

## 3. 目录结构

```
personal_blog/
├─ design.md / plan.md
├─ docker-compose.yml
├─ Dockerfile                  # node build → go build → nginx+二进制
├─ deploy/
│  ├─ nginx.conf               # auth_request 逐请求校验 + 托管 dist
│  └─ entrypoint.sh            # 单容器起双进程并转发信号
│     封禁页/冷却页由 Go 渲染（见 internal/handler/page.go），样式全内联，
│     因此不再需要静态 banned.html。
├─ backend/
│  ├─ go.mod                   # module personal_blog
│  ├─ cmd/server/main.go
│  ├─ tests/                   # 测试程序：限流封禁 / 认证 / HTTP 集成
│  └─ internal/
│     ├─ config/               # env 配置
│     ├─ model/                # User / InviteCode / BanRecord
│     ├─ store/                # GORM(SQLite WAL) + Redis
│     ├─ security/             # argon2id
│     ├─ httpx/                # Cookie / 统一响应
│     ├─ middleware/           # ipguard / auth / csrf
│     ├─ service/              # auth / session / guard / invite
│     ├─ handler/              # auth / admin / internal / page
│     └─ router/               # 路由唯一注册点
└─ frontend/
   └─ src/
      ├─ theme/                # 时段取色 palette.ts + useTimeTheme.ts
      ├─ pages/                # Welcome / Auth / Admin / main(MainShell+Sidebar+sections)
      ├─ components/           # Orbs / ui(Field, ErrorBanner, Spinner)
      ├─ styles/global.css     # 全部颜色走 CSS 变量
      └─ api/client.ts         # fetch 封装 + CSRF + 错误码映射
```

## 4. 数据模型

```
User        id, account(uniq), password_hash(argon2id), nickname,
            role(admin|user), last_ip, created_at
InviteCode  code(uniq), created_by, used_by, expires_at, used_at
BanRecord   ip(uniq), offense_count, level(1|2), expires_at(NULL=永久),
            reason, created_at, updated_at
Session     Redis: sess:<id> → user_id, TTL 7d 滑动续期
```

## 5. 接口清单

**公开（唯一接受输入处，受 ipguard 保护）**

| 方法 | 路径 | 输入 |
|---|---|---|
| POST | `/api/auth/register` | account, password, nickname, inviteCode（必填、单次、7 天有效） |
| POST | `/api/auth/login` | account, password |
| POST | `/api/auth/logout` | — |
| GET | `/api/auth/session` | — |
| GET | `/api/internal/ipcheck` | — （Nginx `auth_request` 专用，仅回环可访问） |

> **闸门协议**：`auth_request` 只把 401/403 当作拒绝，其余非 2xx 会被当成 500，
> 所以闸门统一返回 403，并用 `X-Block: banned|cooldown` 区分，由 Nginx 决定跳向哪一页。

**需登录**：`GET /api/me` → id / account / nickname / ip / role（**无输入**）

**admin（4.3 的白名单例外，需 admin 会话 + CSRF）**

| 方法 | 路径 |
|---|---|
| GET / POST / DELETE | `/api/admin/invites` |
| GET / POST / DELETE | `/api/admin/bans` |
| GET | `/api/admin/users` |
| PATCH | `/api/admin/users/:id/role` |

> 不提供 `/api/features`——功能页是 mock，前端常量即可。

## 6. 认证与冷启动

- Cookie：`session=<id>`，`HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=7d`。
- 会话只存 `user_id`；账号/昵称/IP 查库返回；**密码永不下发**。
- 密码 argon2id。用户名 3–20 位 `[A-Za-z0-9_]`，密码 ≥8 位。
- 冷启动：env `ADMIN_ACCOUNT` / `ADMIN_PASSWORD` 不存在则创建 admin，已存在则跳过。

## 7. 时段主题（`design.md` 2.1 的实现）

8 时段锚点色 + 按分钟线性插值（无跳变），取浏览器本地时间、每 60s 刷新，注入 CSS 变量。
夜间允许转深色，玻璃参数（blur 15px / rgba 白 0.15 / 1px 边框）保持不变。

## 8. 容器拓扑

```
app   ← 唯一暴露端口 8088；内含 nginx:80 + go:127.0.0.1:8081
      ← 卷 ./data/blog.db (SQLite WAL)
redis ← AOF 持久化（永久封禁名单依赖它做计数，名单本身落 SQLite）
```

Nginx 对**每个**请求（含 css/js/图片）走 `auth_request → /api/internal/ipcheck`；
封禁则重定向到 `/banned`、冷却则重定向到 `/cooldown`，这两个页面自身豁免校验。

> ⚠️ **不要在 Nginx 上加 `limit_req`**：若 Nginx 先以 503 拒绝，Go 的滑动窗口就永远数不到
> 20 次，「再犯永久封禁」会直接失效。阈值判定只放在 Go 里。

> ⚠️ **`/api/auth/login` 与 `/api/auth/register` 必须豁免闸门**（`location =` 精确匹配）。
> 这两个接口在冷却期内必须继续被 Go 统计到，否则窗口内永远凑不满 20 次。
> 它们的限流与封禁判定由 Go 的 `ipguard` 中间件负责（封禁 403 / 冷却 429 且仍计数）。
> 这条是实测踩出来的：未豁免时 20 次请求里只有前 3 次到达 Go，第 20 次永远是 302。

> ⚠️ 必须设 `absolute_redirect off;`：容器内监听 80、对外映射到其他端口时，
> Nginx 默认生成的绝对地址会丢掉端口号（跳向 `http://host/banned` 而不是 `http://host:8088/banned`）。

## 9. 实施进度

1. ~~设计系统落盘~~ → 2. ~~`plan.md` + `design.md` 修订~~ → 3. ~~后端骨架与数据层~~
→ 4. ~~认证与 Redis 会话~~ → 5. ~~限流封禁中间件~~ → 6. ~~admin 接口~~
→ 7. ~~前端时段主题与欢迎页动画~~ → 8. ~~Main 滚轮三页~~ → 9. ~~admin 后台页面~~
→ 10. ~~Docker 编排~~ → 11. ~~测试与构建验证~~

### 验证命令

```bash
# 后端静态检查与测试（含 HTTP 集成测试）
cd backend && go vet ./... && go test ./tests/... -count=1

# 前端类型检查与构建
cd frontend && npm run build

# 整体启动（需 Docker Desktop 已运行）
docker compose up -d --build
```
