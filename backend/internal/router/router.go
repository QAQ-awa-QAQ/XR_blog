package router

import (
	"github.com/gin-gonic/gin"

	"personal_blog/internal/config"
	"personal_blog/internal/handler"
	"personal_blog/internal/middleware"
	"personal_blog/internal/service"
)

type Deps struct {
	Config       config.Config
	Auth         *service.Auth
	Sessions     *service.Sessions
	Guard        *service.Guard
	AuthHandler  *handler.AuthHandler
	AdminHandler *handler.AdminHandler
}

// New 组装全部路由。这里是唯一的注册点，方便一眼看清攻击面：
// 只有登录/注册对上暴露输入，其余一律只读或需管理员权限。
func New(d Deps) (*gin.Engine, error) {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 目前直连测试，未来经反代/CF 时只需改 TRUSTED_PROXIES 环境变量即可拿到真实 IP。
	if err := r.SetTrustedProxies(d.Config.TrustedProxies); err != nil {
		return nil, err
	}

	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })
	r.GET("/banned", handler.BannedPage(d.Guard))
	r.GET("/cooldown", handler.CooldownPage(d.Guard))

	requireAuth := middleware.RequireAuth(d.Auth, d.Sessions, d.Config.CookieSecure)

	api := r.Group("/api")

	// design.md 4.3：只在这里接受输入，且必须先过限流/封禁闸门。
	api.POST("/auth/register", middleware.IPGuard(d.Guard), d.AuthHandler.Register)
	api.POST("/auth/login", middleware.IPGuard(d.Guard), d.AuthHandler.Login)
	api.POST("/auth/logout", d.AuthHandler.Logout)

	// 供 Nginx auth_request 调用，仅回环可访问。
	api.GET("/internal/ipcheck", handler.IPCheck(d.Guard))

	authed := api.Group("", requireAuth)
	authed.GET("/auth/session", d.AuthHandler.Session)
	authed.GET("/me", d.AuthHandler.Me)

	// 管理员专属写接口：4.3 白名单例外，双重保护（角色 + CSRF）。
	admin := authed.Group("/admin", middleware.RequireAdmin(), middleware.CSRF())
	{
		admin.GET("/invites", d.AdminHandler.ListInvites)
		admin.POST("/invites", d.AdminHandler.CreateInvite)
		admin.DELETE("/invites/:id", d.AdminHandler.RevokeInvite)

		admin.GET("/bans", d.AdminHandler.ListBans)
		admin.POST("/bans", d.AdminHandler.CreateBan)
		admin.POST("/bans/:ip/unban", d.AdminHandler.Unban)
		admin.DELETE("/bans/:ip", d.AdminHandler.ResetBan)

		admin.GET("/users", d.AdminHandler.ListUsers)
		admin.PATCH("/users/:id/role", d.AdminHandler.SetRole)
	}

	return r, nil
}
