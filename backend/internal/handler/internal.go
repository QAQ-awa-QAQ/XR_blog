package handler

import (
	"net"
	"net/http"

	"github.com/gin-gonic/gin"

	"personal_blog/internal/service"
)

// IPCheck 专供 Nginx auth_request 调用，是所有请求（含静态资源）的封禁闸门。
//
// Nginx 的 auth_request 只把 401/403 当成"拒绝"，其余非 2xx 会被视为 500，
// 所以这里统一返回 403，并用 X-Block 头区分封禁与冷却，由 Nginx 决定跳向哪一页：
//
//	200                     → 放行
//	403 + X-Block: banned   → 已封禁
//	403 + X-Block: cooldown → 冷却期内
//
// 只接受回环地址调用（Nginx 与本进程同容器），避免被外部直接探测。
func IPCheck(g *service.Guard) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isLoopback(c.Request.RemoteAddr) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		ctx := c.Request.Context()
		ip := c.ClientIP()

		if rec, err := g.ActiveBan(ctx, ip); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		} else if rec != nil {
			c.Header("X-Block", "banned")
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		if g.Cooldown(ctx, ip) > 0 {
			c.Header("X-Block", "cooldown")
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Status(http.StatusOK)
	}
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
