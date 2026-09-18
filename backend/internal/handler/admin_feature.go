package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"personal_blog/internal/httpx"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

// FeatureAdminHandler 功能入口的管理接口（挂在 admin 组：
// 已由 RequireAuth + RequireAdmin + CSRF 三重保护）。
type FeatureAdminHandler struct {
	features *service.Features
}

func NewFeatureAdminHandler(features *service.Features) *FeatureAdminHandler {
	return &FeatureAdminHandler{features: features}
}

// adminFeatureDTO 管理端视图：含内网地址（仅 admin 接口下发）。
type adminFeatureDTO struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
	Tag   string `json:"tag"`
	Icon  string `json:"icon"`
	URL   string `json:"url"`
	Sort  int    `json:"sort"`
}

// publicFeatureDTO 公开视图：**绝不含 URL**。
type publicFeatureDTO struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
	Tag   string `json:"tag"`
	Icon  string `json:"icon"`
}

func toAdminFeatureDTO(item model.Feature) adminFeatureDTO {
	return adminFeatureDTO{
		Key: item.Key, Title: item.Title, Desc: item.Desc, Tag: item.Tag, Icon: item.Icon, URL: item.URL, Sort: item.Sort,
	}
}

// PublicList 主页功能入口的公开列表：任何访客都能拿到展示文案，
// 但不含内网地址、不含任何权限信息（响应可被缓存也无风险）。
func (h *FeatureAdminHandler) PublicList(c *gin.Context) {
	items, err := h.features.List(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "读取功能列表失败")
		return
	}
	out := make([]publicFeatureDTO, 0, len(items))
	for _, item := range items {
		out = append(out, publicFeatureDTO{
			Key: item.Key, Title: item.Title, Desc: item.Desc, Tag: item.Tag, Icon: item.Icon,
		})
	}
	httpx.OK(c, gin.H{"features": out})
}

// ListAll 管理端列表：含地址与排序。
func (h *FeatureAdminHandler) ListAll(c *gin.Context) {
	items, err := h.features.List(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "读取功能列表失败")
		return
	}
	out := make([]adminFeatureDTO, 0, len(items))
	for _, item := range items {
		out = append(out, toAdminFeatureDTO(item))
	}
	httpx.OK(c, gin.H{"features": out})
}

func (h *FeatureAdminHandler) Create(c *gin.Context) {
	var body service.FeatureInput
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "请求体格式不正确")
		return
	}
	item, err := h.features.Create(c.Request.Context(), body)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	httpx.OK(c, gin.H{"feature": toAdminFeatureDTO(*item)})
}

func (h *FeatureAdminHandler) Update(c *gin.Context) {
	key := c.Param("key")
	if !service.ValidFeatureKey(key) {
		httpx.Fail(c, http.StatusBadRequest, "invalid_param", "功能标识不合法")
		return
	}
	var body service.FeatureInput
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "请求体格式不正确")
		return
	}
	if err := h.features.Update(c.Request.Context(), key, body); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.Fail(c, http.StatusNotFound, "not_found", "功能不存在")
			return
		}
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *FeatureAdminHandler) Delete(c *gin.Context) {
	key := c.Param("key")
	if !service.ValidFeatureKey(key) {
		httpx.Fail(c, http.StatusBadRequest, "invalid_param", "功能标识不合法")
		return
	}
	if err := h.features.Delete(c.Request.Context(), key); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.Fail(c, http.StatusNotFound, "not_found", "功能不存在")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, "internal_error", "删除功能失败")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *FeatureAdminHandler) Reorder(c *gin.Context) {
	var body struct {
		Keys []string `json:"keys"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "请求体格式不正确")
		return
	}
	if err := h.features.Reorder(c.Request.Context(), body.Keys); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}
