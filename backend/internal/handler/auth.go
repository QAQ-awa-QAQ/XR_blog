package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"personal_blog/internal/config"
	"personal_blog/internal/httpx"
	"personal_blog/internal/middleware"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

type AuthHandler struct {
	auth     *service.Auth
	sessions *service.Sessions
	cfg      config.Config
}

func NewAuthHandler(auth *service.Auth, sessions *service.Sessions, cfg config.Config) *AuthHandler {
	return &AuthHandler{auth: auth, sessions: sessions, cfg: cfg}
}

// Register 注册必须携带有效邀请码；成功后直接建立会话，省去二次登录。
func (h *AuthHandler) Register(c *gin.Context) {
	var body struct {
		Account    string `json:"account" binding:"required"`
		Password   string `json:"password" binding:"required"`
		Nickname   string `json:"nickname" binding:"required"`
		InviteCode string `json:"inviteCode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "请填写账号、密码、昵称与邀请码")
		return
	}

	user, err := h.auth.Register(c.Request.Context(), service.RegisterInput{
		Account:    body.Account,
		Password:   body.Password,
		Nickname:   body.Nickname,
		InviteCode: body.InviteCode,
		IP:         c.ClientIP(),
	})
	if err != nil {
		writeAuthError(c, err)
		return
	}
	h.issueSession(c, user)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var body struct {
		Account  string `json:"account" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "请填写账号与密码")
		return
	}

	user, err := h.auth.Login(c.Request.Context(), body.Account, body.Password, c.ClientIP())
	if err != nil {
		writeAuthError(c, err)
		return
	}
	h.issueSession(c, user)
}

// Guest 游客入口：无凭据、无用户行，只建立一个只读会话。
// 路由上同样挂 IPGuard——它会创建服务端会话，必须防脚本狂刷。
func (h *AuthHandler) Guest(c *gin.Context) {
	ctx := c.Request.Context()

	sid, err := h.sessions.CreateGuest(ctx)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "创建会话失败，请稍后再试")
		return
	}
	csrf, err := randomToken()
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "创建会话失败，请稍后再试")
		return
	}

	httpx.SetSession(c, sid, h.cfg.SessionTTL, h.cfg.CookieSecure)
	httpx.SetCSRF(c, csrf, h.cfg.SessionTTL, h.cfg.CookieSecure)
	httpx.OK(c, gin.H{"user": toUserDTO(model.GuestUser(c.ClientIP())), "csrfToken": csrf})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	if sid, err := c.Cookie(httpx.SessionCookie); err == nil && sid != "" {
		h.sessions.Destroy(c.Request.Context(), sid)
	}
	httpx.ClearSession(c, h.cfg.CookieSecure)
	httpx.OK(c, gin.H{"ok": true})
}

// Session 供前端在进入 main 前确认登录态。
func (h *AuthHandler) Session(c *gin.Context) {
	httpx.OK(c, gin.H{"user": toUserDTO(middleware.CurrentUser(c))})
}

// Me 返回完整登录态：唯一 ID、账号、昵称、IP（密码除外）。
func (h *AuthHandler) Me(c *gin.Context) {
	httpx.OK(c, toUserDTO(middleware.CurrentUser(c)))
}

// issueSession 创建会话并下发 session（HttpOnly）+ csrf（可读）两个 Cookie。
func (h *AuthHandler) issueSession(c *gin.Context, user *model.User) {
	ctx := c.Request.Context()

	sid, err := h.sessions.Create(ctx, user.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "创建登录会话失败，请稍后再试")
		return
	}
	csrf, err := randomToken()
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "创建登录会话失败，请稍后再试")
		return
	}

	httpx.SetSession(c, sid, h.cfg.SessionTTL, h.cfg.CookieSecure)
	httpx.SetCSRF(c, csrf, h.cfg.SessionTTL, h.cfg.CookieSecure)
	httpx.OK(c, gin.H{"user": toUserDTO(user), "csrfToken": csrf})
}

func writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAccountTaken):
		httpx.Fail(c, http.StatusConflict, "account_taken", "该账号已被注册")
	case errors.Is(err, service.ErrInviteInvalid):
		httpx.Fail(c, http.StatusBadRequest, "invite_invalid", "邀请码无效、已使用或已过期")
	case errors.Is(err, service.ErrBadAccount), errors.Is(err, service.ErrBadPassword), errors.Is(err, service.ErrBadNickname):
		httpx.Fail(c, http.StatusBadRequest, "invalid_credentials_format", err.Error())
	case errors.Is(err, service.ErrBadCredentials):
		httpx.Fail(c, http.StatusUnauthorized, "bad_credentials", "账号或密码错误")
	default:
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "服务器开小差了，请稍后再试")
	}
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
