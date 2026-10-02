package handler

import (
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"personal_blog/internal/config"
	"personal_blog/internal/httpx"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

// GatewayHandler 中转网关：把「服务子域请求」在完成会话鉴权与逐功能授权后，
// 反向代理到功能配置的上游地址。
//
// 部署形态：Nginx 用一个独立端口（如 8808）承载本网关——穿透侧把
// cvat.example.com 等域名都指向该端口。上游服务零暴露：
// 未登录 / 游客 / 未授权者连服务页面都看不到，只会得到一张拦截页。
// 浏览器打开的地址固定为域名，不随局域网 / 穿透等访问方式变化。
//
// Nginx 把请求以 /_gw 前缀送入（rewrite ^ /_gw$uri break），
// 与博客自身的 /api 路由彻底隔离——子域下的任意路径（包括它自己的 /api/…）
// 都不会误入博客接口。
type GatewayHandler struct {
	features *service.Features
	sessions *service.Sessions
	auth     *service.Auth
	access   *service.Access
	cfg      config.Config
}

func NewGatewayHandler(
	features *service.Features,
	sessions *service.Sessions,
	auth *service.Auth,
	access *service.Access,
	cfg config.Config,
) *GatewayHandler {
	return &GatewayHandler{features: features, sessions: sessions, auth: auth, access: access, cfg: cfg}
}

// NormalizeHost 请求 Host 归一化：小写、去端口、去尾点。
func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]") // 裸 IPv6 字面量兜底
	return strings.TrimSuffix(host, ".")
}

// Serve 处理网关入口的每一类请求（HTTP 与 WebSocket 同路径）。
func (h *GatewayHandler) Serve(c *gin.Context) {
	ctx := c.Request.Context()

	// 还原原始路径；Nginx 已做 URI 规范化，path.Clean 为纵深防御。
	raw := c.Param("path")
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	reqPath := path.Clean(raw)

	host := NormalizeHost(c.Request.Host)
	if host == "" {
		h.renderBlock(c, http.StatusBadRequest, "请求缺少主机名，无法确定要访问的服务。")
		return
	}

	// 1) 按 Host 认领功能
	feat, err := h.features.GetByPublicHost(ctx, host)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.renderBlock(c, http.StatusNotFound, "该地址没有配置任何中转服务。")
		return
	}
	if err != nil {
		h.renderBlock(c, http.StatusInternalServerError, "服务器开小差了，请稍后再试。")
		return
	}

	// 2) 会话鉴权（与后台一致：滑动续期、失效即清 Cookie）
	sid, cookieErr := c.Cookie(httpx.SessionCookie)
	if cookieErr != nil || sid == "" {
		h.renderLoginNeeded(c)
		return
	}
	uid, ok := h.sessions.UserID(ctx, sid)
	if !ok {
		httpx.ClearSession(c, h.cfg.CookieSecure, h.cfg.CookieDomain)
		h.renderLoginNeeded(c)
		return
	}

	var user *model.User
	if uid == 0 {
		user = model.GuestUser(c.ClientIP())
	} else {
		u, err := h.auth.FindByID(ctx, uid)
		if err != nil {
			h.sessions.Destroy(ctx, sid)
			httpx.ClearSession(c, h.cfg.CookieSecure, h.cfg.CookieDomain)
			h.renderLoginNeeded(c)
			return
		}
		user = u
	}
	h.sessions.Touch(ctx, sid)

	// 3) 逐功能授权
	if user.Role == model.RoleGuest {
		h.renderBlock(c, http.StatusForbidden, "游客无法访问中转服务，请登录正式账号后重试。")
		return
	}
	if user.Role != model.RoleAdmin {
		allowed, err := h.access.CanOpen(ctx, user.ID, feat.Key)
		if err != nil {
			h.renderBlock(c, http.StatusInternalServerError, "服务器开小差了，请稍后再试。")
			return
		}
		if !allowed {
			h.renderBlock(c, http.StatusForbidden, "您没有权限访问该服务，请向管理页申请。")
			return
		}
	}

	// 4) 上游地址
	target := strings.TrimSpace(feat.URL)
	if target == "" {
		h.renderBlock(c, http.StatusNotFound, "该服务尚未配置上游地址。")
		return
	}
	upstream, err := url.Parse(target)
	if err != nil || (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.Host == "" {
		h.renderBlock(c, http.StatusBadGateway, "上游地址配置不正确，请检查功能配置。")
		return
	}

	// 5) 放行：反向代理（WebSocket 由标准库原生透传）
	h.proxy(c, upstream, reqPath)
}

// proxy 将请求反代到上游：路径拼接、剥离博客凭据、重写自身的内网跳转地址。
func (h *GatewayHandler) proxy(c *gin.Context, upstream *url.URL, reqPath string) {
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = upstream.Scheme
			pr.Out.URL.Host = upstream.Host
			pr.Out.URL.Path = joinURLPath(upstream.Path, reqPath)
			pr.Out.URL.RawPath = ""
			// 上游看到的目标 Host 就是它自己（很多应用按 Host 校验）；
			// 对外域名通过 X-Forwarded-Host 传给上游。
			pr.Out.Host = upstream.Host

			// 剥离博客自己的会话凭据，其余 Cookie 原样透传。
			// （上游自己的 cookie 与博客互不干扰：各自作用在独立子域。）
			stripGatewayCookies(pr.Out)

			if pr.In.Header.Get("X-Forwarded-Proto") == "" {
				pr.Out.Header.Set("X-Forwarded-Proto", schemeOf(pr.In))
			}
			pr.Out.Header.Set("X-Forwarded-Host", NormalizeHost(pr.In.Host))
		},
		ModifyResponse: func(resp *http.Response) error {
			// 上游返回的绝对跳转若指向它自己的内网地址，改写为对外域名，
			// 否则浏览器会拿到 127.0.0.1 这类不可达地址。
			loc := resp.Header.Get("Location")
			if loc == "" {
				return nil
			}
			base := upstream.Scheme + "://" + upstream.Host
			if !strings.HasPrefix(loc, base) {
				return nil
			}
			proto := resp.Request.Header.Get("X-Forwarded-Proto")
			host := resp.Request.Header.Get("X-Forwarded-Host")
			if proto == "" || host == "" {
				return nil
			}
			resp.Header.Set("Location", proto+"://"+host+strings.TrimPrefix(loc, base))
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			writeGatewayPage(w, http.StatusBadGateway, pageData{
				Badge:   "Gateway error",
				Title:   "中转服务暂不可达",
				Message: "上游服务没有响应（可能未启动或地址有误），请稍后再试。",
			})
		},
	}
	rp.ServeHTTP(c.Writer, c.Request)
}

// stripGatewayCookies 转发前去掉博客自身的 Cookie（其余全部保留）。
func stripGatewayCookies(req *http.Request) {
	cookies := req.Cookies()
	if len(cookies) == 0 {
		return
	}
	req.Header.Del("Cookie")
	for _, ck := range cookies {
		if ck.Name == httpx.SessionCookie || ck.Name == httpx.CSRFCookie {
			continue
		}
		req.AddCookie(ck)
	}
}

// joinURLPath 拼接上游 base 路径与请求路径（请求路径以 / 开头）。
func joinURLPath(base, p string) string {
	if base == "" || base == "/" {
		return p
	}
	return strings.TrimSuffix(base, "/") + p
}

func schemeOf(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// ---------- 拦截页 ----------

func (h *GatewayHandler) renderLoginNeeded(c *gin.Context) {
	data := pageData{
		Badge:   "Login required",
		Title:   "请先登录",
		Message: "该服务位于中转网关之后：请先用管理员或已授权账号在博客登录，再回到本地址访问。",
	}
	if h.cfg.BlogPublicURL != "" {
		data.Link = h.cfg.BlogPublicURL
		data.LinkText = "前往博客登录"
	}
	h.writePage(c, http.StatusUnauthorized, data)
}

func (h *GatewayHandler) renderBlock(c *gin.Context, status int, message string) {
	h.writePage(c, status, pageData{
		Badge:   "Proxy gateway",
		Title:   "访问被拦截",
		Message: message,
	})
}

func (h *GatewayHandler) writePage(c *gin.Context, status int, data pageData) {
	c.Data(status, "text/html; charset=utf-8", []byte(renderPage(data)))
}

// writeGatewayPage 供 ReverseProxy 的 ErrorHandler 使用（那里只有 http.ResponseWriter）。
func writeGatewayPage(w http.ResponseWriter, status int, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(renderPage(data)))
}
