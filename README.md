# XR 个人站

一个个人博客 / 名片站：整屏滚动的欢迎页动画、随本地时间连续变色的玻璃质感界面，加上一套带邀请码与限流封禁的多用户鉴权系统。

前端用 React + TypeScript 手写交互与动画，后端是单二进制的 Go 服务，整个站点打包成**一个镜像**（Nginx + Go 双进程）即可部署。

---

## 特性

**界面**

- 欢迎页是一条 GSAP 时间轴：默认色幕布（跟随系统浅/深）渐变到站点背景 → 中央 logo 弹性浮现
  → logo 左移，标题从 logo 后面逐字「钻出」→ 箭头按钮从末字下方滑出，按钮内画线成「→」
- 点击后「→」渐变为转圈（双向过渡，不是瞬切）；登录成功接一段整页过场：欢迎页左移出，
  箭头克隆飞向侧栏「首页」按钮并转向（桌面旋成 ↓ / 手机保持 →）后发射淡出，
  侧栏按钮依次弹出，正文再沿**垂直于侧栏**的方向弹出（桌面从左侧横向 / 手机从下方纵向）
- 主题色随**本地时间**在 8 个时段之间按分钟连续插值，夜间自动转深色；文字色是**离散两档**（按背景亮度切换），保证任何时刻对比度达标
- 「更多」页可改主题偏好：**跟随时间**（默认开）或拖滑块把主题固定到某个时段
  （滑块轨道直接铺这一天的 8 段锚点色）。偏好落 `localStorage`，刷新后还记得
- iOS 27 液态玻璃：`feTurbulence` + `feDisplacementMap` 让背景**真的被玻璃挤歪**（不只是模糊），外缘带一圈主题色柔光；带 `url()` 的声明配一条不带 `url()` 的回退，不支持的浏览器上退化成普通毛玻璃而不是失效
- 主页面**双轴向**：内容装得下就走纵向整屏切换；装不下自动转手机逻辑（横向切换 + 正文纵向滚动）
- 切换由**临界阻尼弹簧**逐帧追赶：连滚会合并成最新目标，途中反向会保留速度改向；收尾有亚像素吸附，不会「停不下来」
- 卡片入场：波包式错开 + 长距离位移，流入方向**跟随翻页方向**；登录后首次进入会减弱幅度
- 侧栏：四个等大的图标页（首页 / 功能 / 联系 / 更多），选中态是**滑动指示条**；按钮只剩图标，标签走 `aria-label` / `title`
- 侧栏的**轨道本身是隐形的**（不画底色 / 边框 / 模糊 / 投影）：能看见的只有那枚玻璃胶囊，和贴着轮廓的「假边线」
  —— 真面板一个像素也不画，改用 `.sidebar__edge` 这一层，并用一个跟随胶囊的 SVG 洞把胶囊盖住的区域挖掉
- 光标扫过时，假边线亮起 1px 硬高光 + 向外弥散的柔光，**光色随明暗反相**（亮色主题深色弥散 +
  灰白描边，深色主题保持亮白）；胶囊把边线掐断的那四个「头」上各挂一个**常亮的虚拟光标**
  （与真光标共用半径 / 落点曲线 / 模糊，唯一差别是没有 `opacity` 门禁）
- 手机端：横条落到底部，**同一套设计转 90°** —— 胶囊躺下、溢出从左右改成上下、四个头随之转置
- 滚动条隐藏（不占位、不影响滚动），尊重 `prefers-reduced-motion`，键盘焦点可见

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
| 测试 | 后端 Go `testing` + miniredis（24 项）；前端 `node --test` 自检 25 项 |

---

## 运动与视觉设计

动画参数集中在 `frontend/src/motion/tokens.ts`，纯函数在 `motion/math.ts`
（不依赖 DOM，可以直接被 `node --test` 导入断言）；自绘控件的设计参数（图标路径、
形变几何、侧栏尺寸口径）在 `frontend/src/design/`，其 `README.md` 记录为什么是这些值。
前端自检用 `npm run check`，已接在 `prebuild` 上，所以 `npm run build` 会先跑完 25 项断言。

关键决策与踩过的坑都记在 `DESIGN-LOG.md`，例如：

- 短程位移不能用 smootherstep：它两端速度归零，10% 时长只走 0.9%，按钮滑动会「先卡住再猛冲」
- 入场的**波包错开不能去掉** —— 它是「层次感」与弹簧感的来源，删了就退化成整块平移
- 轴判定不能看 `scrollHeight`：滚动区域会把 `transform` 造成的溢出算进去，入场位移会把轴误判成横向
- 入场目标元素**不能挂 CSS `transform` 过渡**：过渡会把 GSAP 写入的起始值拦下来
- 文字色必须离散两档：插值会让某个时刻的前景/背景对比度归零
- 液态玻璃的折射只能靠 SVG 位移滤镜，而且**必须先挤后模糊**：先模糊就没什么可挤的了。
  另外带 `url()` 的 `backdrop-filter` 要另写一条不带 `url()` 的声明当回退 ——
  否则不支持的浏览器会把整条丢掉，玻璃直接变全透明
- 用 `mask` 抠形状时，`mask-clip` 默认是 `border-box`，**它会把元素的绘制内容锁死在自己的盒子里**：
  想靠放大 `mask-size` 把光画到盒子外是白做（实测外侧亮度差 ≈ 0），得把承载它的盒子本身撑出去
- 自定义属性里的 `var()` 是在**声明它的那个元素**上求值的，不是惰性的：
  把引用 `--cut-*` 的 token 挂到 `:root`，会被当场冻成 fallback（亮点定格在角落、不跟胶囊走）
- 径向渐变只写首尾两个停靠点时，CSS 是**平分插值**的 —— 出来的是一条直线衰减；
  要曲线就把中间几站显式摆上（这里用 α ∝ 1-√t）

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
>
> ⚠️ `docker-compose.yml` 里的默认口令 `123` 是**本地模拟用的弱口令**，且会随仓库公开。
> 对外部署前务必用环境变量覆盖：`ADMIN_PASSWORD='强密码' docker compose up -d`。

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

## 目录结构

```
backend/           Go 服务（cmd/server、internal/{handler,middleware,service,store}、tests）
frontend/
  src/design/      自绘控件的设计参数（图标路径、形变几何）与其口径说明
  src/motion/      运动数学与参数（纯函数，可被 node --test 直接断言）
  src/theme/       按本地时间连续变色的主题
  src/content/     站点文案（当前仍是占位内容）
  src/pages/main/  主页面：双轴向切换、侧栏、四个板块
  scripts/         自检脚本 check-{content,motion,theme}.mjs
  reference/       主题对照页（人工核对色板用）
deploy/            Nginx 配置与容器入口脚本
design.md          需求与设计文档
plan.md            实施步骤
DESIGN-LOG.md      设计决策与踩坑记录
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

前端有一组**纯函数断言**（25 项），把动效与配色里靠肉眼难查的性质固定下来：

```bash
cd frontend
npm run check          # 内容 / 运动数学 / 时段主题；已接入 prebuild，构建前自动执行
npm run check:theme    # 单独跑某一组
```

- `check-content`：文案结构、长度上限、链接与邮箱格式、占位标记
- `check-motion`：缓动两端速度为零、临界阻尼不超调且可被打断接续、波包幅度有界、
  呼吸周期互质（8s/13s/26s/40s 都不是周期）、光斑位移与缩放不越界
- `check-theme`：锚点精确命中、逐分钟连续、夜间明显更暗、**任何时刻文字对比度都足够**

前端类型检查与构建：

```bash
cd frontend && npm run build     # tsc --noEmit + vite build（build 前自动跑上面的断言）
```

### 开发用对照页

配色与动效的参数都在 `frontend/reference/` 下有对应的**实时对照页**（只在开发服务器存在，不进入生产构建）：

```bash
cd frontend && npm run dev
then open  http://localhost:5173/reference/palette-review.html
```

拖动时间轴即可查看任意时刻的插值结果，并直接读出「相邻分钟最大通道差」「文字色一天切换几次」
「最差对比度出现在哪一刻」——不用改代码再刷新。

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
├─ frontend/
│  ├─ src/
│  │  ├─ theme/                 # 时段取色（palette + useTimeTheme）
│  │  ├─ motion/                # 运动数学：缓动 / 弹簧 / 波包 / 光斑
│  │  ├─ design/                # 自绘控件的设计参数（图标路径、形变几何）+ 口径说明
│  │  ├─ content/site.ts        # 全站文案（与组件分离）
│  │  ├─ pages/                 # Welcome / Auth / Admin / main / Handoff
│  │  ├─ components/            # Orbs / Field / ErrorBanner
│  │  └─ api/client.ts
│  ├─ scripts/                  # 三组校验脚本（node --test）
│  └─ reference/                # 开发用对照页（不进生产构建）
├─ deploy/
│  ├─ nginx.conf                # 闸门 + 静态托管
│  └─ entrypoint.sh             # 单容器双进程
├─ design-system/               # 由 ui-ux-pro-max 生成的设计系统
├─ design.md                    # 需求（唯一需求源）
├─ DESIGN-LOG.md                # 决策日志：数值依据与**被否决的方案**
└─ plan.md                      # 实施方案与踩坑记录
```

---

## 实现要点

几个容易踩坑、已在 `plan.md` 与 `DESIGN-LOG.md` 中记录的设计决定：

- **Nginx 闸门返回 403 + `X-Block: banned|cooldown`**。`auth_request` 只把 401/403 当作拒绝，返回 429 会被当成 500，因此用响应头区分两种拦截原因，由 `@blocked` 决定跳向哪一页。
- **登录/注册两个接口必须豁免闸门**。冷却期内它们仍要被 Go 统计到，否则 20 次阈值永远凑不满，「再犯永久封禁」会变成死规则。
- **不要在 Nginx 上加 `limit_req`**。Nginx 若先返回 503，Go 的滑动窗口同样数不到 20 次。阈值判定只放在 Go 里。
- **`absolute_redirect off`**。容器内监听 80、对外映射到其他端口时，默认的绝对地址会丢掉端口号。
- **文字色不参与插值**。背景由浅变深、文字由深变浅，两者同时连续插值**必然**在中途亮度相等，
  那一瞬间对比度趋近于 0（实测约 10，文字基本不可读）。改为按背景亮度**离散切换两档**，
  阈值放在亮度中点，最差时刻仍有 110 以上的亮度差。
- **整屏切换用临界阻尼弹簧，不用「事件 + 时间锁」**。滚轮只更新目标值，画面逐帧追过去，
  所以连续快滚会被合并成最新目标（实测间隔 150ms 连滚三次可直达第三页），
  运动途中反向也能保留速度改向。
- **主页面交互轴按内容自动切换**。装得下时纵向整屏切换（滚轮 / 上下键 / 上下滑，正文不滚动）；
  装不下时转为横向（左右滑 / 左右键 / 横向滚轮），正文纵向自己滚。两种模式下都不会出现页面滚动条。

---

## 已知限制

- 首页 / 功能 / 联系 / 更多四页文案与功能入口为**占位内容**，等待替换
- 暂未提供修改密码接口，改密需重建数据库或新增接口
- 未内置 HTTPS，生产部署请在外部反向代理或平台上终止 TLS，并把 `COOKIE_SECURE` 设为 `true`

---

## 许可

本项目采用 [MIT License](./LICENSE)。
