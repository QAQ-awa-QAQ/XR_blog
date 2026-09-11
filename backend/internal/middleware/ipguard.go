package middleware

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"personal_blog/internal/httpx"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

// IPGuard 只挂在登录/注册两个接口上，实现 design.md 第 4 章：
//  1. 已达阈值 → 封禁（首次 24h，再犯永久）
//  2. 10s 内 3 次 → 冷却 10s，期间一律 429
//  3. 冷却期内仍然计数 —— 否则 20 次阈值永远达不到
func IPGuard(g *service.Guard) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ip := c.ClientIP()

		// 已封禁的 IP 直接拒绝，避免仅靠窗口计数漏判。
		if rec, err := g.ActiveBan(ctx, ip); err != nil {
			log.Printf("[ipguard] 查询封禁记录失败 ip=%s err=%v", ip, err)
		} else if rec != nil {
			abortBanned(c, g, rec)
			return
		}

		verdict, rec, cooldown, err := g.Attempt(ctx, ip)
		if err != nil {
			// Redis 故障时 fail-open，避免把正常用户整体锁在门外；故障会落日志。
			log.Printf("[ipguard] 计数失败 ip=%s err=%v", ip, err)
			c.Next()
			return
		}

		switch verdict {
		case service.VerdictBanned:
			abortBanned(c, g, rec)
		case service.VerdictCooldown:
			retry := int(cooldown.Seconds()) + 1
			c.Header("Retry-After", strconv.Itoa(retry))
			httpx.Fail(c, http.StatusTooManyRequests, "cooldown",
				"操作过于频繁，请 "+strconv.Itoa(retry)+" 秒后再试")
		default:
			c.Next()
		}
	}
}

func abortBanned(c *gin.Context, g *service.Guard, rec *model.BanRecord) {
	s := g.Summary(rec)
	body := gin.H{
		"code":      "banned",
		"message":   "你涉嫌网络攻击已被封禁",
		"permanent": s.Permanent,
		"reason":    s.Reason,
	}
	if s.Permanent {
		c.Header("Retry-After", "86400")
	} else if s.Remaining > 0 {
		seconds := int(s.Remaining.Seconds()) + 1
		c.Header("Retry-After", strconv.Itoa(seconds))
		body["retry_after"] = seconds
	}
	c.AbortWithStatusJSON(http.StatusForbidden, body)
}
