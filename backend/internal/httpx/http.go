package httpx

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	SessionCookie = "session"
	CSRFCookie    = "csrf"
	CSRFHeader    = "X-CSRF-Token"
	CtxUserKey    = "currentUser"
)

// SetSession 下发会话 Cookie：HttpOnly，避免 XSS 读取。
// domain 非空时（如 .example.com）用于中转网关的跨子域共享；空则 host-only。
func SetSession(c *gin.Context, id string, ttl time.Duration, secure bool, domain string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(SessionCookie, id, int(ttl.Seconds()), "/", domain, secure, true)
}

func ClearSession(c *gin.Context, secure bool, domain string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(SessionCookie, "", -1, "/", domain, secure, true)
}

// SetCSRF 下发 CSRF token。必须可被前端 JS 读取，故 HttpOnly=false，
// admin 写接口以 double-submit 方式校验。
// 注意：刻意不带 Domain —— 保持 host-only，让服务子域页面读不到它。
func SetCSRF(c *gin.Context, token string, ttl time.Duration, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CSRFCookie, token, int(ttl.Seconds()), "/", "", secure, false)
}

// OK 统一成功响应：直接返回数据体。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// Fail 统一失败响应，code 供前端分支判断。
func Fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"code": code, "message": message})
}
