package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"personal_blog/internal/model"
)

// Access 访问控制域：用户组、组 × 功能授权。
//
// 安全约定：功能地址只存服务端（见 feature.go），
// 点击时按「管理员全通 / 普通用户看组 / 游客拒绝」判定后才下发。
type Access struct{ db *gorm.DB }

func NewAccess(db *gorm.DB) *Access { return &Access{db: db} }

// GroupView 后台展示用：组 + 授权功能 keys + 成员用户 ID。
type GroupView struct {
	ID       uint     `json:"id"`
	Name     string   `json:"name"`
	Features []string `json:"features"`
	Members  []uint   `json:"members"`
}

var featureKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// ValidFeatureKey 功能标识白名单：小写字母/数字/连字符，最长 64。
// 路由参数、库字段都先过这一关。
func ValidFeatureKey(key string) bool { return featureKeyPattern.MatchString(key) }

// ---------- 组与授权 ----------

// CanOpen 判断用户是否有权打开某功能：命中任一所隶属组的授权即可。
func (a *Access) CanOpen(ctx context.Context, userID uint, key string) (bool, error) {
	var count int64
	err := a.db.WithContext(ctx).Model(&model.GroupFeature{}).
		Joins("JOIN user_groups ON user_groups.group_id = group_features.group_id").
		Where("user_groups.user_id = ? AND group_features.feature_key = ?", userID, key).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListGroups 全部组（含授权与成员），供管理后台一次性拉取。
func (a *Access) ListGroups(ctx context.Context) ([]GroupView, error) {
	var groups []model.Group
	if err := a.db.WithContext(ctx).Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	var feats []model.GroupFeature
	if err := a.db.WithContext(ctx).Find(&feats).Error; err != nil {
		return nil, err
	}
	var members []model.UserGroup
	if err := a.db.WithContext(ctx).Find(&members).Error; err != nil {
		return nil, err
	}

	featMap := make(map[uint][]string, len(groups))
	for _, f := range feats {
		featMap[f.GroupID] = append(featMap[f.GroupID], f.FeatureKey)
	}
	memMap := make(map[uint][]uint, len(groups))
	for _, m := range members {
		memMap[m.GroupID] = append(memMap[m.GroupID], m.UserID)
	}

	out := make([]GroupView, 0, len(groups))
	for _, g := range groups {
		features := featMap[g.ID]
		if features == nil {
			features = []string{}
		}
		mem := memMap[g.ID]
		if mem == nil {
			mem = []uint{}
		}
		out = append(out, GroupView{ID: g.ID, Name: g.Name, Features: features, Members: mem})
	}
	return out, nil
}

func validateGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 16 {
		return "", errors.New("组名需为 1-16 个字符")
	}
	return name, nil
}

// CreateGroup 新建用户组（初始无授权、无成员）。
func (a *Access) CreateGroup(ctx context.Context, name string) (*model.Group, error) {
	name, err := validateGroupName(name)
	if err != nil {
		return nil, err
	}
	g := model.Group{Name: name}
	if err := a.db.WithContext(ctx).Create(&g).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, errors.New("该组名已存在")
		}
		return nil, err
	}
	return &g, nil
}

// UpdateGroup 更新组名与/或授权集合（features 覆盖式；nil 表示不改）。
func (a *Access) UpdateGroup(ctx context.Context, id uint, name *string, features *[]string) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var g model.Group
		if err := tx.First(&g, "id = ?", id).Error; err != nil {
			return err
		}
		if name != nil {
			n, err := validateGroupName(*name)
			if err != nil {
				return err
			}
			if err := tx.Model(&g).Update("name", n).Error; err != nil {
				if isUniqueViolation(err) {
					return errors.New("该组名已存在")
				}
				return err
			}
		}
		if features != nil {
			if err := tx.Delete(&model.GroupFeature{}, "group_id = ?", id).Error; err != nil {
				return err
			}
			for _, key := range *features {
				if !ValidFeatureKey(key) {
					return errors.New("非法的功能标识：" + key)
				}
				if err := tx.Create(&model.GroupFeature{GroupID: id, FeatureKey: key}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// DeleteGroup 删除组并清理授权与成员关联。
func (a *Access) DeleteGroup(ctx context.Context, id uint) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&model.Group{}, "id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Delete(&model.GroupFeature{}, "group_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(&model.UserGroup{}, "group_id = ?", id).Error
	})
}

// SetMembers 覆盖式设置组成员（只接受真实存在的用户）。
func (a *Access) SetMembers(ctx context.Context, id uint, userIDs []uint) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var g model.Group
		if err := tx.First(&g, "id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.UserGroup{}, "group_id = ?", id).Error; err != nil {
			return err
		}
		for _, uid := range userIDs {
			var count int64
			if err := tx.Model(&model.User{}).Where("id = ?", uid).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return errors.New("用户不存在")
			}
			if err := tx.Create(&model.UserGroup{UserID: uid, GroupID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
