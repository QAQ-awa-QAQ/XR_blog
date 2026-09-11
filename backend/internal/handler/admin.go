package handler

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"personal_blog/internal/httpx"
	"personal_blog/internal/middleware"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

// AdminHandler 是 design.md 4.3 的"白名单例外"：管理员专属写接口。
type AdminHandler struct {
	invite *service.Invite
	guard  *service.Guard
	auth   *service.Auth
}

func NewAdminHandler(invite *service.Invite, guard *service.Guard, auth *service.Auth) *AdminHandler {
	return &AdminHandler{invite: invite, guard: guard, auth: auth}
}

// ---------- 邀请码 ----------

type inviteDTO struct {
	ID        uint      `json:"id"`
	Code      string    `json:"code"`
	CreatedBy uint      `json:"createdBy"`
	UsedBy    *uint     `json:"usedBy"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

func (h *AdminHandler) ListInvites(c *gin.Context) {
	items, err := h.invite.List(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "读取邀请码失败")
		return
	}

	now := time.Now()
	out := make([]inviteDTO, 0, len(items))
	for _, it := range items {
		status := "available"
		switch {
		case it.Used():
			status = "used"
		case it.Expired(now):
			status = "expired"
		}
		out = append(out, inviteDTO{
			ID:        it.ID,
			Code:      it.Code,
			CreatedBy: it.CreatedBy,
			UsedBy:    it.UsedBy,
			Status:    status,
			ExpiresAt: it.ExpiresAt,
			CreatedAt: it.CreatedAt,
		})
	}
	httpx.OK(c, gin.H{"invites": out})
}

func (h *AdminHandler) CreateInvite(c *gin.Context) {
	user := middleware.CurrentUser(c)
	invite, err := h.invite.Create(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "生成邀请码失败")
		return
	}
	httpx.OK(c, inviteDTO{
		ID:        invite.ID,
		Code:      invite.Code,
		CreatedBy: invite.CreatedBy,
		Status:    "available",
		ExpiresAt: invite.ExpiresAt,
		CreatedAt: invite.CreatedAt,
	})
}

func (h *AdminHandler) RevokeInvite(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.invite.Revoke(c.Request.Context(), id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.Fail(c, http.StatusNotFound, "not_found", "邀请码不存在或已被使用")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "撤销邀请码失败")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ---------- 封禁 ----------

type banDTO struct {
	IP           string     `json:"ip"`
	OffenseCount int        `json:"offenseCount"`
	Permanent    bool       `json:"permanent"`
	Active       bool       `json:"active"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	Reason       string     `json:"reason"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func toBanDTO(rec model.BanRecord, now time.Time) banDTO {
	return banDTO{
		IP:           rec.IP,
		OffenseCount: rec.OffenseCount,
		Permanent:    rec.Level == model.BanLevelPermanent,
		Active:       rec.Active(now),
		ExpiresAt:    rec.ExpiresAt,
		Reason:       rec.Reason,
		UpdatedAt:    rec.UpdatedAt,
	}
}

func (h *AdminHandler) ListBans(c *gin.Context) {
	records, err := h.guard.ListBans(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "读取封禁名单失败")
		return
	}
	now := time.Now()
	out := make([]banDTO, 0, len(records))
	for _, rec := range records {
		out = append(out, toBanDTO(rec, now))
	}
	httpx.OK(c, gin.H{"bans": out})
}

func (h *AdminHandler) CreateBan(c *gin.Context) {
	var body struct {
		IP        string `json:"ip" binding:"required"`
		Reason    string `json:"reason"`
		Permanent bool   `json:"permanent"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "请提供要封禁的 IP")
		return
	}
	body.IP = strings.TrimSpace(body.IP)
	if net.ParseIP(body.IP) == nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_ip", "IP 地址格式不正确")
		return
	}
	if body.Reason == "" {
		body.Reason = "管理员手动封禁"
	}

	rec, err := h.guard.MarkBanned(c.Request.Context(), body.IP, body.Reason, body.Permanent)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "封禁失败")
		return
	}
	httpx.OK(c, toBanDTO(*rec, time.Now()))
}

func (h *AdminHandler) Unban(c *gin.Context) {
	ip := c.Param("ip")
	if err := h.guard.Unban(c.Request.Context(), ip); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.Fail(c, http.StatusNotFound, "not_found", "该 IP 没有封禁记录")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "解封失败")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ResetBan 彻底清除记录，触犯次数归零。
func (h *AdminHandler) ResetBan(c *gin.Context) {
	ip := c.Param("ip")
	if err := h.guard.Reset(c.Request.Context(), ip); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.Fail(c, http.StatusNotFound, "not_found", "该 IP 没有封禁记录")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "清除记录失败")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ---------- 用户 ----------

func (h *AdminHandler) ListUsers(c *gin.Context) {
	users, err := h.auth.ListUsers(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "读取用户列表失败")
		return
	}
	httpx.OK(c, gin.H{"users": toUserDTOs(users)})
}

func (h *AdminHandler) SetRole(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "缺少 role 字段")
		return
	}
	if err := h.auth.SetRole(c.Request.Context(), id, body.Role); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.Fail(c, http.StatusNotFound, "not_found", "用户不存在")
			return
		}
		httpx.Fail(c, http.StatusBadRequest, "invalid_role", err.Error())
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || v == 0 {
		httpx.Fail(c, http.StatusBadRequest, "invalid_param", "参数 "+name+" 不合法")
		return 0, false
	}
	return uint(v), true
}
