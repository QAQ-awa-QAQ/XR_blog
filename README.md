# XR 个人站

一个个人博客 / 名片站：整屏滚动的欢迎页动画、随本地时间连续变色的玻璃质感界面，加上一套带邀请码与限流封禁的多用户鉴权系统。

前端用 React + TypeScript 手写交互与动画，后端是单二进制的 Go 服务，整个站点打包成**一个镜像**（Nginx + Go 双进程）即可部署。

---

## 特性

**界面**

- 欢迎页 GSAP 动画：水滴玻璃浮出 → 文字逐字凝聚 → 「登录 →」浮现，可点击跳过
- 主题色随**本地时间**在 8 个时段（凌晨/清晨/上午/中午/下午/傍晚/晚上/午夜）之间按分钟线性插值，夜间自动转深色，全程无跳变
- Liquid Glass 视觉：`backdrop-filter` 毛玻璃、1px 高光边框、多层光斑背景
- 整屏切换：滚轮 / 触控板 / 方向键 / PageUp-Down / Home-End / 触屏滑动，侧栏三个入口（简介 · 功能 · 联系），每页背景动画各不相同
- 响应式（桌面优先，移动端可用），尊重 `prefers-reduced-motion`，键盘焦点可见

**账号与权限**

- 邀请码注册（必填、一次性、7 天有效），argon2id 哈希存储
- Redis 会话 + HttpOnly Cookie，7 天滑动续期
- 角色区分：`admin` 可访问 `/admin` 管理后台；管理接口需管理员会话 + CSRF 双重校验

**安全（`design.md` 第 4 章的可执行实现）**

| 规则 | 触发条件 | 动作 |
|---|---|---|
| 冷却 | 同 IP 10 秒内 ≥3 次登录/注册 | 返回 429 并进入 10 秒冷却，**冷却期内仍继续计数** |
| 封禁 | 同 IP 20 秒内 ≥20 次 | 首次 **24 小时**，再犯 **永久** |
| 拦截 | 任意请求（含 css/js/图片） | 重定向到封禁页 / 冷却页 |

- 计数用 Redis ZSET 毫秒级滑动窗口；封禁名单落 SQLite，`offense_count` 永不清除
- 封禁页与冷却页由 Go 渲染，样式全部内联、零外部资源请求
- 除登录/注册与管理员写接口外，**不接受任何输入**（无评论、无留言、无搜索）

---

## 技术栈

| 层 | 选型 |
|---|---|
| 前端 | React 19 · Vite 7 · TypeScript · GSAP |
| 后端 | Go · Gin · GORM · SQLite（纯 Go driver，免 CGO） |
| 缓存/会话 | Redis 7（AOF 持久化） |
| 部署 | 单容器内 Nginx + Go 双进程，多阶段构建 |
| 测试 | Go `testing` + miniredis（无需真实 Redis） |

---

## 快速开始

### Docker（推荐）

```bash
git clone https://github.com/QAQ-awa-QAQ/XR_blog.git
cd XR_blog

docker compose up -d --build
```

打开 http://localhost:8080 即可。

首次启动会自动创建管理员账号，**随机密码只打印一次**，用下面的命令查看：

```bash
docker compose logs app | Select-String "初始密码"     # PowerShell
docker compose logs app | grep "初始密码"              # bash
```

也可以用环境变量预先指定：

```bash
ADMIN_ACCOUNT=admin ADMIN_PASSWORD='your-strong-password' docker compose up -d --build
```

> `ADMIN_PASSWORD` 仅在账号**不存在**时生效，改它不会更新已有密码。

### 本地开发

需要本机有 Redis（默认 `127.0.0.1:6379`）。

```bash
# 后端
cd backend
go run ./cmd/server          # 监听 127.0.0.1:8081

# 前端（另开一个终端）
cd frontend
npm install
npm run dev                  # 5173，/api 自动代理到 8081
```

---

## 首次使用流程

1. 用管理员账号登录（见上方日志里的密码）
2. 进入 `/admin` → 「生成邀请码」，复制 16 位邀请码
3. 退出，在欢迎页点「登录 →」→ 切到「注册」标签，填入邀请码完成注册

后台可以管理邀请码、封禁名单（手动封禁 / 解封 / 清除记录）与用户角色。

---

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `ADDR` | `:8081` | Go 监听地址（容器内只监听回环，由 Nginx 反代） |
| `DB_PATH` | `data/blog.db` | SQLite 文件路径 |
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis 地址 |
| `SESSION_TTL` | `168h` | 会话有效期（滑动续期） |
| `COOKIE_SECURE` | `true` | 明文 HTTP 部署需设为 `false`，挂 HTTPS 后改回 `true` |
| `TRUSTED_PROXIES` | `127.0.0.1,::1` | 可信代理，决定如何取真实客户端 IP |
| `ADMIN_ACCOUNT` | `admin` | 冷启动管理员账号 |
| `ADMIN_PASSWORD` | 空 | 留空则随机生成并打印到日志 |
| `COOLDOWN_WINDOW` / `COOLDOWN_LIMIT` / `COOLDOWN_DURATION` | `10s` / `3` / `10s` | 冷却规则 |
| `BAN_WINDOW` / `BAN_LIMIT` / `BAN_DURATION` | `20s` / `20` / `24h` | 封禁规则 |

真实客户端 IP 取自 `RemoteAddr`（边缘直连场景）。前面再加一层反向代理或 Cloudflare 时，只需启用 Nginx `realip` 模块、声明可信网段并同步调整 `TRUSTED_PROXIES`，应用代码无需改动。

---

## 接口一览

**公开（唯一接受输入处，受限流保护）**

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/register` | 账号 + 密码 + 昵称 + 邀请码 |
| POST | `/api/auth/login` | 账号 + 密码 |
| POST | `/api/auth/logout` | 销毁会话 |
| GET | `/api/auth/session` | 查询当前登录态 |
| GET | `/api/internal/ipcheck` | 内部闸门，仅回环可访问 |

**需登录**：`GET /api/me`、`GET /api/auth/session`

**管理员（需 CSRF 头 `X-CSRF-Token`）**

| 方法 | 路径 |
|---|---|
| GET / POST / DELETE | `/api/admin/invites`、`/api/admin/invites/:id` |
| GET / POST | `/api/admin/bans` |
| POST / DELETE | `/api/admin/bans/:ip/unban`、`/api/admin/bans/:ip` |
| GET / PATCH | `/api/admin/users`、`/api/admin/users/:id/role` |

---

## 测试

```bash
cd backend
go vet ./...
go test ./tests/... -count=1
```

`backend/tests/` 覆盖：冷却与封禁阈值、冷却期仍计数、再犯永久、限时封禁到期保留触犯次数、argon2id 哈希与校验、邀请码必填/单次/过期、重复账号、会话生命周期、冷启动管理员只建一次，以及走完整路由与中间件链的 HTTP 集成测试（错误码、管理员角色 + CSRF、闸门 `X-Block` 协议）。

前端构建校验：

```bash
cd frontend && npm run build     # tsc --noEmit + vite build
```

---

## 目录结构

```
.
├─ backend/
│  ├─ cmd/server/            # 入口
│  ├─ internal/
│  │  ├─ config/             # 环境变量
│  │  ├─ model/              # User / InviteCode / BanRecord
│  │  ├─ store/              # SQLite(WAL) + Redis
│  │  ├─ security/           # argon2id
│  │  ├─ httpx/              # Cookie 与统一响应
│  │  ├─ middleware/         # ipguard / auth / csrf
│  │  ├─ service/            # auth / session / guard / invite
│  │  ├─ handler/            # auth / admin / internal / page
│  │  └─ router/             # 路由唯一注册点
│  └─ tests/                 # 测试程序
├─ frontend/src/
│  ├─ theme/                 # 时段取色（核心视觉逻辑）
│  ├─ pages/                 # Welcome / Auth / Admin / main
│  ├─ components/            # Orbs / Field / ErrorBanner
│  └─ api/client.ts
├─ deploy/
│  ├─ nginx.conf             # 闸门 + 静态托管
│  └─ entrypoint.sh          # 单容器双进程
├─ design-system/            # 由 ui-ux-pro-max 生成的设计系统
├─ design.md                 # 需求（唯一需求源）
└─ plan.md                   # 实施方案与踩坑记录
```

---

## 实现要点

几个容易踩坑、已在 `plan.md` 中记录的设计决定：

- **Nginx 闸门返回 403 + `X-Block: banned|cooldown`**。`auth_request` 只把 401/403 当作拒绝，返回 429 会被当成 500，因此用响应头区分两种拦截原因，由 `@blocked` 决定跳向哪一页。
- **登录/注册两个接口必须豁免闸门**。冷却期内它们仍要被 Go 统计到，否则 20 次阈值永远凑不满，「再犯永久封禁」会变成死规则。
- **不要在 Nginx 上加 `limit_req`**。Nginx 若先返回 503，Go 的滑动窗口同样数不到 20 次。阈值判定只放在 Go 里。
- **`absolute_redirect off`**。容器内监听 80、对外映射到其他端口时，默认的绝对地址会丢掉端口号。

---

## 已知限制

- 简介 / 功能 / 联系三页文案与功能入口为**占位内容**，等待替换
- 暂未提供修改密码接口，改密需重建数据库或新增接口
- 未内置 HTTPS，生产部署请在外部反向代理或平台上终止 TLS，并把 `COOKIE_SECURE` 设为 `true`

---

## 许可

本项目采用 [MIT License](./LICENSE)。
