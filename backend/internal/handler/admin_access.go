package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"personal_blog/internal/httpx"
	"personal_blog/internal/service"
)

// AccessAdminHandler 用户组 / 功能地址的管理接口（挂在 admin 组：
// 已由 RequireAuth + RequireAdmin + CSRF 三重保护）。
type AccessAdminHandler struct {
	access *service.Access
}

func NewAccessAdminHandler(access *service.Access) *AccessAdminHandler {
	return &AccessAdminHandler{access: access}
}

// ---------- 用户组 ----------

func (h *AccessAdminHandler) ListGroups(c *gin.Context) {
	groups, err := h.access.ListGroups(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "读取用户组失败")
		return
	}
	httpx.OK(c, gin.H{"groups": groups})
}

func (h *AccessAdminHandler) CreateGroup(c *gin.Context) {
	var body struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "请填写组名")
		return
	}
	group, err := h.access.CreateGroup(c.Request.Context(), body.Name)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	httpx.OK(c, gin.H{"group": service.GroupView{
		ID: group.ID, Name: group.Name, Features: []string{}, Members: []uint{},
	}})
}

func (h *AccessAdminHandler) UpdateGroup(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body struct {
		Name     *string   `json:"name"`
		Features *[]string `json:"features"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "请求体格式不正确")
		return
	}
	if body.Name == nil && body.Features == nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "没有需要更新的字段")
		return
	}
	if err := h.access.UpdateGroup(c.Request.Context(), id, body.Name, body.Features); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			httpx.Fail(c, http.StatusNotFound, "not_found", "用户组不存在")
		default:
			httpx.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *AccessAdminHandler) DeleteGroup(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.access.DeleteGroup(c.Request.Context(), id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.Fail(c, http.StatusNotFound, "not_found", "用户组不存在")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "删除用户组失败")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *AccessAdminHandler) SetMembers(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body struct {
		UserIDs []uint `json:"userIds"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "请求体格式不正确")
		return
	}
	if err := h.access.SetMembers(c.Request.Context(), id, body.UserIDs); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			httpx.Fail(c, http.StatusNotFound, "not_found", "用户组不存在")
		default:
			httpx.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}
