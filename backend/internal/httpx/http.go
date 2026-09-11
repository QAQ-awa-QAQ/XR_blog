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
func SetSession(c *gin.Context, id string, ttl time.Duration, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(SessionCookie, id, int(ttl.Seconds()), "/", "", secure, true)
}

func ClearSession(c *gin.Context, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(SessionCookie, "", -1, "/", "", secure, true)
}

// SetCSRF 下发 CSRF token。必须可被前端 JS 读取，故 HttpOnly=false，
// admin 写接口以 double-submit 方式校验。
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
