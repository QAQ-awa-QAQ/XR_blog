package router

import (
	"github.com/gin-gonic/gin"

	"personal_blog/internal/config"
	"personal_blog/internal/handler"
	"personal_blog/internal/middleware"
	"personal_blog/internal/service"
)

type Deps struct {
	Config         config.Config
	Auth           *service.Auth
	Sessions       *service.Sessions
	Guard          *service.Guard
	Access         *service.Access
	Features       *service.Features
	AuthHandler    *handler.AuthHandler
	AdminHandler   *handler.AdminHandler
	AccessHandler  *handler.AccessAdminHandler
	FeatureHandler *handler.FeatureAdminHandler
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

	// 主页功能入口的公开列表：只有展示字段（标题/描述/标签/图标），
	// 不含地址、不含权限——任何访客都能读，与挂在 bundle 里的静态数据等价。
	api.GET("/features", d.FeatureHandler.PublicList)

	// design.md 4.3：只在这里接受输入，且必须先过限流/封禁闸门。
	// 游客入口不接输入，但会创建服务端会话，同样纳入限流防刷。
	api.POST("/auth/register", middleware.IPGuard(d.Guard), d.AuthHandler.Register)
	api.POST("/auth/login", middleware.IPGuard(d.Guard), d.AuthHandler.Login)
	api.POST("/auth/guest", middleware.IPGuard(d.Guard), d.AuthHandler.Guest)
	// 登出同样要求 double-submit CSRF：防跨站强制登出（低危，但前端本就携带 token）
	api.POST("/auth/logout", middleware.CSRF(), d.AuthHandler.Logout)

	// 供 Nginx auth_request 调用，仅回环可访问。
	api.GET("/internal/ipcheck", handler.IPCheck(d.Guard))

	authed := api.Group("", requireAuth)
	authed.GET("/auth/session", d.AuthHandler.Session)
	authed.GET("/me", d.AuthHandler.Me)

	// 功能入口的点击鉴权：登录态 + CSRF；判定全在服务端，
	// 游客同样吃 403（前端对游客不注册点击，这里是兜底）。
	authed.POST("/features/:key/open", middleware.CSRF(), handler.FeatureOpen(d.Features, d.Access))

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

		// 用户组：建组 / 改名 / 勾选授权功能 / 挑选成员。
		admin.GET("/groups", d.AccessHandler.ListGroups)
		admin.POST("/groups", d.AccessHandler.CreateGroup)
		admin.PATCH("/groups/:id", d.AccessHandler.UpdateGroup)
		admin.DELETE("/groups/:id", d.AccessHandler.DeleteGroup)
		admin.PUT("/groups/:id/members", d.AccessHandler.SetMembers)

		// 功能入口：增删改 + 拖拽排序（内网地址只存服务端，按权限下发）。
		admin.GET("/features", d.FeatureHandler.ListAll)
		admin.POST("/features", d.FeatureHandler.Create)
		admin.PATCH("/features/:key", d.FeatureHandler.Update)
		admin.DELETE("/features/:key", d.FeatureHandler.Delete)
		admin.PUT("/features/order", d.FeatureHandler.Reorder)
	}

	return r, nil
}
