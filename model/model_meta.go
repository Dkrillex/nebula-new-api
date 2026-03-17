package model

import (
	"fmt"
	"one-api/common"
	"one-api/setting/ratio_setting"
	"strconv"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const (
	NameRuleExact = iota
	NameRulePrefix
	NameRuleContains
	NameRuleSuffix
)

type BoundChannel struct {
	Name string `json:"name"`
	Type int    `json:"type"`
}

type Model struct {
	Id            int            `json:"id"`
	ModelName     string         `json:"model_name" gorm:"size:128;not null;uniqueIndex:uk_model_name_delete_at,priority:1"`
	ModelNickName string         `json:"model_nick_name,omitempty" gorm:"type:varchar(128)"`
	Description   string         `json:"description,omitempty" gorm:"type:text"`
	DescriptionEn string         `json:"description_en,omitempty" gorm:"type:text"`
	DescriptionId string         `json:"description_id,omitempty" gorm:"type:text"`
	Icon          string         `json:"icon,omitempty" gorm:"type:varchar(128)"`
	IconURL       string         `json:"icon_url,omitempty" gorm:"type:varchar(128)"`
	Tags          string         `json:"tags,omitempty" gorm:"type:varchar(255)"`
	TagsEn        string         `json:"tags_en,omitempty" gorm:"type:varchar(255)"`
	TagsId        string         `json:"tags_id,omitempty" gorm:"type:varchar(255)"`
	ShowTab       *int           `json:"show_tab,omitempty" gorm:"type:int"`
	ModelLimit    string         `json:"model_limit,omitempty" gorm:"type:text"`
	VendorID      int            `json:"vendor_id,omitempty" gorm:"index"`
	Endpoints     string         `json:"endpoints,omitempty" gorm:"type:text"`
	Status        int            `json:"status" gorm:"default:1"`
	Flag          int            `json:"flag" gorm:"default:1"`       // 模型广场展示标识：0-无 1-新发布 2-最先进 3-火爆
	SortOrder     int            `json:"sort_order" gorm:"default:1"` // 排序值，越小优先级越高
	SyncOfficial  int            `json:"sync_official" gorm:"default:1"`
	CreatedTime   int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime   int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index;uniqueIndex:uk_model_name_delete_at,priority:2"`

	BoundChannels []BoundChannel `json:"bound_channels,omitempty" gorm:"-"`
	EnableGroups  []string       `json:"enable_groups,omitempty" gorm:"-"`
	QuotaTypes    []int          `json:"quota_types,omitempty" gorm:"-"`
	NameRule      int            `json:"name_rule" gorm:"default:0"`

	MatchedModels []string `json:"matched_models,omitempty" gorm:"-"`
	MatchedCount  int      `json:"matched_count,omitempty" gorm:"-"`
}

func (mi *Model) Insert() error {
	now := common.GetTimestamp()
	mi.CreatedTime = now
	mi.UpdatedTime = now
	err := DB.Create(mi).Error
	if err == nil {
		// 数据库操作成功，异步删除并立即更新缓存，确保下次访问时获取最新数据
		// 使用 recover 确保缓存更新失败不影响主业务逻辑
		gopool.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					common.SysLog("failed to refresh cache after model insert: " + fmt.Sprintf("%v", r))
				}
			}()
			ratio_setting.RefreshExposedDataCache()
			RefreshPricing()
		})
	}
	return err
}

func IsModelNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	var cnt int64
	err := DB.Model(&Model{}).Where("model_name = ? AND id <> ?", name, id).Count(&cnt).Error
	return cnt > 0, err
}

func (mi *Model) Update() error {
	mi.UpdatedTime = common.GetTimestamp()
	updates := map[string]interface{}{
		"model_name":      mi.ModelName,
		"model_nick_name": mi.ModelNickName,
		"description":     mi.Description,
		"description_en":  mi.DescriptionEn,
		"description_id":  mi.DescriptionId,
		"icon":            mi.Icon,
		"icon_url":        mi.IconURL,
		"tags":            mi.Tags,
		"tags_en":         mi.TagsEn,
		"tags_id":         mi.TagsId,
		"show_tab":        mi.ShowTab,
		"model_limit":     mi.ModelLimit,
		"vendor_id":       mi.VendorID,
		"endpoints":       mi.Endpoints,
		"status":          mi.Status,
		"sync_official":   mi.SyncOfficial,
		"updated_time":    mi.UpdatedTime,
		"name_rule":       mi.NameRule,
		"flag":            mi.Flag,
		"sort_order":      mi.SortOrder,
	}
	err := DB.Model(&Model{}).Where("id = ?", mi.Id).Updates(updates).Error
	if err == nil {
		// 数据库操作成功，异步删除并立即更新缓存，确保下次访问时获取最新数据
		// 使用 recover 确保缓存更新失败不影响主业务逻辑
		gopool.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					common.SysLog("failed to refresh cache after model update: " + fmt.Sprintf("%v", r))
				}
			}()
			ratio_setting.RefreshExposedDataCache()
			RefreshPricing()
		})
	}
	return err
}

func (mi *Model) Delete() error {
	err := DB.Delete(mi).Error
	if err == nil {
		// 数据库操作成功，异步删除并立即更新缓存，确保下次访问时获取最新数据
		// 使用 recover 确保缓存更新失败不影响主业务逻辑
		gopool.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					common.SysLog("failed to refresh cache after model delete: " + fmt.Sprintf("%v", r))
				}
			}()
			ratio_setting.RefreshExposedDataCache()
			RefreshPricing()
		})
	}
	return err
}

func GetVendorModelCounts() (map[int64]int64, error) {
	var stats []struct {
		VendorID int64
		Count    int64
	}
	if err := DB.Model(&Model{}).
		Select("vendor_id as vendor_id, count(*) as count").
		Group("vendor_id").
		Scan(&stats).Error; err != nil {
		return nil, err
	}
	m := make(map[int64]int64, len(stats))
	for _, s := range stats {
		m[s.VendorID] = s.Count
	}
	return m, nil
}

func GetAllModels(offset int, limit int, status *int) ([]*Model, error) {
	var models []*Model
	db := DB.Model(&Model{})
	if status != nil {
		db = db.Where("status = ?", *status)
	}
	err := db.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error
	return models, err
}

func GetBoundChannelsByModelsMap(modelNames []string) (map[string][]BoundChannel, error) {
	result := make(map[string][]BoundChannel)
	if len(modelNames) == 0 {
		return result, nil
	}
	type row struct {
		Model string
		Name  string
		Type  int
	}
	var rows []row
	err := DB.Table("channels").
		Select("abilities.model as model, channels.name as name, channels.type as type").
		Joins("JOIN abilities ON abilities.channel_id = channels.id").
		Where("abilities.model IN ? AND abilities.enabled = ?", modelNames, true).
		Distinct().
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.Model] = append(result[r.Model], BoundChannel{Name: r.Name, Type: r.Type})
	}
	return result, nil
}

func SearchModels(keyword string, vendor string, offset int, limit int, status *int) ([]*Model, int64, error) {
	var models []*Model
	db := DB.Model(&Model{})
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("model_name LIKE ? OR description LIKE ? OR description_en LIKE ? OR description_id LIKE ? OR tags LIKE ? OR tags_en LIKE ? OR tags_id LIKE ?", like, like, like, like, like, like, like)
	}
	if vendor != "" {
		if vid, err := strconv.Atoi(vendor); err == nil {
			db = db.Where("models.vendor_id = ?", vid)
		} else {
			db = db.Joins("JOIN vendors ON vendors.id = models.vendor_id").Where("vendors.name LIKE ?", "%"+vendor+"%")
		}
	}
	if status != nil {
		db = db.Where("models.status = ?", *status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := db.Order("models.id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	return models, total, nil
}
