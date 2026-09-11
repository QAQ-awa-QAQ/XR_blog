package handler

import (
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"personal_blog/internal/service"
)

// 封禁页与冷却页由 Go 直接渲染：样式全部内联在文档里，零外部资源请求，
// 这样被封禁的 IP 也能看到完整页面（Nginx 已豁免这两个路径的 auth_request）。

const pageStyle = `
:root{color-scheme:light}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:grid;place-items:center;
font-family:"Inter","PingFang SC","Microsoft YaHei",system-ui,-apple-system,sans-serif;
background:linear-gradient(160deg,#FAFAFA 0%,#EEF2F7 45%,#E8ECF0 100%);
color:#09090B;overflow:hidden}
.orb{position:fixed;border-radius:50%;filter:blur(60px);opacity:.55;pointer-events:none}
.orb-1{width:46vmax;height:46vmax;left:-12vmax;top:-14vmax;background:radial-gradient(circle at 30% 30%,#F9A8D4,#EC4899 60%,transparent 72%)}
.orb-2{width:38vmax;height:38vmax;right:-10vmax;bottom:-12vmax;background:radial-gradient(circle at 60% 40%,#BAE6FD,#60A5FA 55%,transparent 70%)}
.card{position:relative;width:min(92vw,520px);padding:40px 36px;border-radius:28px;
background:rgba(255,255,255,.24);border:1px solid rgba(255,255,255,.55);
backdrop-filter:blur(18px) saturate(180%);-webkit-backdrop-filter:blur(18px) saturate(180%);
box-shadow:0 24px 60px rgba(15,23,42,.16),inset 0 1px 0 rgba(255,255,255,.7);text-align:center}
.badge{display:inline-block;padding:6px 14px;border-radius:999px;font-size:12px;font-weight:600;
letter-spacing:.08em;text-transform:uppercase;background:rgba(220,38,38,.12);color:#B91C1C;
border:1px solid rgba(220,38,38,.28)}
h1{margin:18px 0 10px;font-size:24px;font-weight:600;line-height:1.45}
p{margin:0;font-size:15px;line-height:1.7;color:rgba(9,9,11,.72)}
.meta{margin-top:22px;padding:14px 16px;border-radius:16px;font-size:13px;
background:rgba(255,255,255,.4);border:1px solid rgba(255,255,255,.6);color:rgba(9,9,11,.66);
display:grid;gap:6px}
.kv{display:flex;justify-content:space-between;gap:16px}
.kv b{font-weight:600;color:rgba(9,9,11,.86)}
code{font-family:ui-monospace,"SFMono-Regular",Menlo,monospace;font-size:12.5px}
`

type pageData struct {
	Badge    string
	Title    string
	Message  string
	Rows     [][2]string
	ShowMeta bool
}

func renderPage(d pageData) string {
	var rows string
	for _, r := range d.Rows {
		rows += fmt.Sprintf(`<div class="kv"><span>%s</span><b><code>%s</code></b></div>`,
			html.EscapeString(r[0]), html.EscapeString(r[1]))
	}
	meta := ""
	if d.ShowMeta && rows != "" {
		meta = `<div class="meta">` + rows + `</div>`
	}
	return `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + html.EscapeString(d.Title) + `</title><style>` + pageStyle + `</style></head>` +
		`<body><div class="orb orb-1"></div><div class="orb orb-2"></div>` +
		`<main class="card"><span class="badge">` + html.EscapeString(d.Badge) + `</span>` +
		`<h1>` + html.EscapeString(d.Title) + `</h1><p>` + html.EscapeString(d.Message) + `</p>` +
		meta + `</main></body></html>`
}

// BannedPage 展示封禁详情与剩余时长（永久封禁不显示倒计时）。
func BannedPage(g *service.Guard) gin.HandlerFunc {
	return func(c *gin.Context) {
		data := pageData{
			Badge:   "Access blocked",
			Title:   "你涉嫌网络攻击已被封禁",
			Message: "该 IP 因在极短时间内发起大量登录或注册请求，已被系统自动拦截。",
		}

		rec, err := g.ActiveBan(c.Request.Context(), c.ClientIP())
		if err == nil && rec != nil {
			s := g.Summary(rec)
			data.Rows = append(data.Rows, [2]string{"IP", s.IP})
			if s.Permanent {
				data.Rows = append(data.Rows, [2]string{"封禁时长", "永久"})
			} else if s.Remaining > 0 {
				data.Rows = append(data.Rows, [2]string{"剩余时间", humanDuration(s.Remaining)})
			}
			if s.Reason != "" {
				data.Rows = append(data.Rows, [2]string{"原因", s.Reason})
			}
			data.ShowMeta = true
		}

		c.Data(http.StatusForbidden, "text/html; charset=utf-8", []byte(renderPage(data)))
	}
}

// CooldownPage 对应 4.1 的 10 秒冷却。
func CooldownPage(g *service.Guard) gin.HandlerFunc {
	return func(c *gin.Context) {
		data := pageData{
			Badge:   "Rate limited",
			Title:   "操作过于频繁，请稍后再试",
			Message: "系统检测到短时间内多次登录或注册请求，已临时限制该 IP 的访问。",
		}
		if cd := g.Cooldown(c.Request.Context(), c.ClientIP()); cd > 0 {
			data.Rows = append(data.Rows, [2]string{"剩余时间", humanDuration(cd)})
			data.ShowMeta = true
		}
		c.Data(http.StatusTooManyRequests, "text/html; charset=utf-8", []byte(renderPage(data)))
	}
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d 秒", int(d.Seconds())+1)
	}
	if d < time.Hour {
		return fmt.Sprintf("%d 分 %d 秒", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%d 小时 %d 分", int(d.Hours()), int(d.Minutes())%60)
}
