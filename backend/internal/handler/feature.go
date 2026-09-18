package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"personal_blog/internal/httpx"
	"personal_blog/internal/middleware"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

// FeatureOpen 功能入口的点击鉴权。
//
// 判定顺序（全部在服务端，前端不持有任何权限信息，改 DOM 也绕不过）：
//  1. 游客角色 → 一律 403（前端对游客不注册点击、不发请求，这里是兜底）；
//  2. 管理员 → 放行；
//  3. 普通用户 → 所隶属任一组授权了该功能才放行，否则 403；
//  4. 通过后才读取内网地址并返回 —— 未授权者连「是否配置过地址」都不透露。
//
// 地址绝不下发到静态产物；响应带 no-store，避免任何中间缓存留存。
func FeatureOpen(features *service.Features, access *service.Access) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		user := middleware.CurrentUser(c)
		key := c.Param("key")

		if user == nil || !service.ValidFeatureKey(key) {
			httpx.Fail(c, http.StatusNotFound, "not_found", "功能不存在")
			return
		}

		if user.Role == model.RoleGuest {
			httpx.Fail(c, http.StatusForbidden, "forbidden", "您没有权限查看，请向管理页申请")
			return
		}

		if user.Role != model.RoleAdmin {
			ok, err := access.CanOpen(ctx, user.ID, key)
			if err != nil {
				httpx.Fail(c, http.StatusInternalServerError, "internal_error", "服务器开小差了，请稍后再试")
				return
			}
			if !ok {
				httpx.Fail(c, http.StatusForbidden, "forbidden", "您没有权限查看，请向管理页申请")
				return
			}
		}

		target, err := features.GetURL(ctx, key)
		if errors.Is(err, service.ErrFeatureNotConfigured) {
			httpx.Fail(c, http.StatusNotFound, "not_configured", "该功能暂未开放")
			return
		}
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, "internal_error", "服务器开小差了，请稍后再试")
			return
		}

		c.Header("Cache-Control", "no-store")
		httpx.OK(c, gin.H{"url": target})
	}
}
