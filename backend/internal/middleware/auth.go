package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"

	"personal_blog/internal/httpx"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

// RequireAuth 校验会话 Cookie 并注入当前用户（同时滑动续期）。
func RequireAuth(auth *service.Auth, sessions *service.Sessions, secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		sid, err := c.Cookie(httpx.SessionCookie)
		if err != nil || sid == "" {
			httpx.Fail(c, http.StatusUnauthorized, "unauthenticated", "请先登录")
			return
		}

		userID, ok := sessions.UserID(ctx, sid)
		if !ok {
			httpx.ClearSession(c, secure)
			httpx.Fail(c, http.StatusUnauthorized, "unauthenticated", "登录已过期，请重新登录")
			return
		}

		// 游客会话（哨兵 0）：注入合成身份，不查库。
		// 它没有 admin 角色，管理接口由 RequireAdmin 兜底拒绝。
		if userID == 0 {
			sessions.Touch(ctx, sid)
			c.Set(httpx.CtxUserKey, model.GuestUser(c.ClientIP()))
			c.Next()
			return
		}

		user, err := auth.FindByID(ctx, userID)
		if err != nil {
			sessions.Destroy(ctx, sid)
			httpx.ClearSession(c, secure)
			httpx.Fail(c, http.StatusUnauthorized, "unauthenticated", "登录已过期，请重新登录")
			return
		}

		sessions.Touch(ctx, sid)
		c.Set(httpx.CtxUserKey, user)
		c.Next()
	}
}

// RequireAdmin 必须挂在 RequireAuth 之后。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if CurrentUser(c) == nil || CurrentUser(c).Role != model.RoleAdmin {
			httpx.Fail(c, http.StatusForbidden, "forbidden", "需要管理员权限")
			return
		}
		c.Next()
	}
}

// CSRF 使用 double-submit cookie：写操作要求请求头与 Cookie 中的 token 一致。
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		cookie, err := c.Cookie(httpx.CSRFCookie)
		header := c.GetHeader(httpx.CSRFHeader)
		if err != nil || cookie == "" || header == "" ||
			subtle.ConstantTimeCompare([]byte(cookie), []byte(header)) != 1 {
			httpx.Fail(c, http.StatusForbidden, "csrf", "CSRF 校验失败，请刷新页面重试")
			return
		}
		c.Next()
	}
}

// CurrentUser 读取 RequireAuth 注入的用户。
func CurrentUser(c *gin.Context) *model.User {
	if v, ok := c.Get(httpx.CtxUserKey); ok {
		if user, ok := v.(*model.User); ok {
			return user
		}
	}
	return nil
}
