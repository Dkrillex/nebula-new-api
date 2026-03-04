package model

import (
	"context"
	"fmt"
	"one-api/common"
	"one-api/logger"
	"one-api/types"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Log struct {
	Id               int    `json:"id" gorm:"index:idx_created_at_id,priority:1"`
	UserId           int    `json:"user_id" gorm:"index"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type             int    `json:"type" gorm:"index:idx_created_at_type"`
	Content          string `json:"content"`
	Username         string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName        string `json:"token_name" gorm:"index;default:''"`
	ModelName        string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota            int    `json:"quota" gorm:"default:0"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	UseTime          int    `json:"use_time" gorm:"default:0"`
	IsStream         bool   `json:"is_stream"`
	ChannelId        int    `json:"channel" gorm:"index"`
	ChannelName      string `json:"channel_name" gorm:"->"`
	TokenId          int    `json:"token_id" gorm:"default:0;index"`
	Group            string `json:"group" gorm:"index"`
	Ip               string `json:"ip" gorm:"index;default:''"`
	Other            string `json:"other"`
	// OEM系统价格链条字段
	OemId          *int64 `json:"oem_id" gorm:"column:oem_id;index"`
	OfficialQuota  int64  `json:"official_quota" gorm:"default:0"`
	CostQuota      int64  `json:"cost_quota" gorm:"default:0"`
	SystemQuota    int64  `json:"system_quota" gorm:"default:0"`
	UserQuota      int64  `json:"user_quota" gorm:"default:0"`
	PlatformProfit int64  `json:"platform_profit" gorm:"default:0"`
	OemSubsidy     int64  `json:"oem_subsidy" gorm:"default:0"`
}

const (
	LogTypeUnknown = iota
	LogTypeTopup
	LogTypeConsume
	LogTypeManage
	LogTypeSystem
	LogTypeError
	LogTypeTopUpAndConsume
)

func formatUserLogs(logs []*Log) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// delete admin
			delete(otherMap, "admin_info")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
	}
}

func GetLogByKey(key string) (logs []*Log, err error) {
	if os.Getenv("LOG_SQL_DSN") != "" {
		var tk Token
		if err = DB.Model(&Token{}).Where(logKeyCol+"=?", strings.TrimPrefix(key, "sk-")).First(&tk).Error; err != nil {
			return nil, err
		}
		err = LOG_DB.Model(&Log{}).Where("token_id=?", tk.Id).Find(&logs).Error
	} else {
		err = LOG_DB.Joins("left join tokens on tokens.id = logs.token_id").Where("tokens.key = ?", strings.TrimPrefix(key, "sk-")).Find(&logs).Error
	}
	formatUserLogs(logs)
	return logs, err
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, content))
	username := c.GetString("username")
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		Other: otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
	// OEM系统价格链条字段
	PriceChain *PriceChainParams `json:"price_chain,omitempty"`
}

// PriceChainParams 价格链条参数
type PriceChainParams struct {
	OemId          *int64 `json:"oem_id"`
	OemCode        string `json:"oem_code"` // 用于向后兼容和日志显示
	OfficialQuota  int64  `json:"official_quota"`
	CostQuota      int64  `json:"cost_quota"`
	SystemQuota    int64  `json:"system_quota"`
	UserQuota      int64  `json:"user_quota"`
	PlatformProfit int64  `json:"platform_profit"`
	OemSubsidy     int64  `json:"oem_subsidy"`
	VendorId       *int64 `json:"vendor_id,omitempty"` // 厂商ID，存于 other 用于账单导出
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	// 合并 other 与 vendor_id（来自 PriceChain），避免突变调用方传入的 map
	other := make(map[string]interface{})
	if params.Other != nil {
		for k, v := range params.Other {
			other[k] = v
		}
	}
	if params.PriceChain != nil && params.PriceChain.VendorId != nil {
		other["vendor_id"] = *params.PriceChain.VendorId
	}
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		Other: otherStr,
	}

	// 记录价格链条信息
	if params.PriceChain != nil {
		log.OemId = params.PriceChain.OemId
		log.OfficialQuota = params.PriceChain.OfficialQuota
		log.CostQuota = params.PriceChain.CostQuota
		log.SystemQuota = params.PriceChain.SystemQuota
		log.UserQuota = params.PriceChain.UserQuota
		log.PlatformProfit = params.PriceChain.PlatformProfit
		log.OemSubsidy = params.PriceChain.OemSubsidy
	}

	// 先插入日志以获取日志ID
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
		return
	}

	// 处理OEM补贴（在日志插入后，可以传入日志ID）
	if params.PriceChain != nil && log.OemSubsidy != 0 && params.PriceChain.OemCode != "" {
		logId := log.Id
		ProcessOemSubsidy(c, params.PriceChain.OemCode, log.OemSubsidy, logId, userId)
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(userId, username, params.ModelName, params.Quota, common.GetTimestamp(), params.PromptTokens+params.CompletionTokens)
		})
	}
}

// ProcessOemSubsidy 处理OEM补贴
// 如果OEM补贴为负数（需要补贴），从系统账户扣除；如果为正数（盈利），增加到系统账户
func ProcessOemSubsidy(c *gin.Context, oemCode string, oemSubsidy int64, logId int, userId int) error {
	if oemSubsidy == 0 {
		common.SysLog(fmt.Sprintf("OEM补贴处理跳过: oemCode=%s, oemSubsidy=0", oemCode))
		return nil
	}

	// 获取OEM配置
	oemConfig := GetOemConfigByCode(oemCode)
	if oemConfig == nil {
		common.SysLog(fmt.Sprintf("OEM补贴处理跳过: oemCode=%s, OEM配置不存在", oemCode))
		return nil
	}
	if oemConfig.OemAdminNebulaApiId == nil {
		// 如果没有配置OEM管理员账户，不处理补贴，但记录日志
		common.SysLog(fmt.Sprintf("OEM补贴处理跳过: oemCode=%s, OemAdminNebulaApiId未配置（请在oem_config表中设置oem_admin_nebula_api_id字段）", oemCode))
		return nil
	}

	systemAccountUserId := int(*oemConfig.OemAdminNebulaApiId)

	// 获取变动前余额
	user, err := GetUserById(systemAccountUserId, false)
	if err != nil {
		common.SysLog(fmt.Sprintf("获取OEM管理员账户失败: systemAccountUserId=%d, error=%v", systemAccountUserId, err))
		return err
	}
	balanceBefore := int64(user.Quota)

	if oemSubsidy < 0 {
		// 需要补贴：从系统账户扣除（oemSubsidy是负数，所以需要取绝对值）
		subsidyAmount := int(-oemSubsidy)
		err := DecreaseUserQuota(systemAccountUserId, subsidyAmount)
		if err != nil {
			common.SysLog(fmt.Sprintf("处理OEM补贴失败: oemCode=%s, systemAccountUserId=%d, subsidyAmount=%d, error=%v",
				oemCode, systemAccountUserId, subsidyAmount, err))
			return err
		}

		// 记录OEM账户变动日志
		balanceAfter := balanceBefore + oemSubsidy
		// 将quota转换为人民币（1美金=500000quota=7.0人民币）
		subsidyRmb := float64(-oemSubsidy) / common.QuotaPerUnit * 7.0
		accountLog := &OemAccountLog{
			OemId:          oemConfig.Id,
			OemAdminUserId: systemAccountUserId,
			CreatedAt:      common.GetTimestamp(),
			Type:           OemAccountLogTypeSubsidy,
			Amount:         oemSubsidy, // 负数
			BalanceBefore:  balanceBefore,
			BalanceAfter:   balanceAfter,
			RelatedLogId:   &logId,
			RelatedUserId:  &userId,
			Content:        fmt.Sprintf("OEM：-¥%.6f", subsidyRmb),
		}
		if err := RecordOemAccountLog(accountLog); err != nil {
			common.SysLog(fmt.Sprintf("记录OEM账户变动日志失败: error=%v", err))
		}

		common.SysLog(fmt.Sprintf("OEM补贴扣除: oemCode=%s, systemAccountUserId=%d, subsidyAmount=%d",
			oemCode, systemAccountUserId, subsidyAmount))
	} else {
		// 盈利：增加到系统账户
		profitAmount := int(oemSubsidy)
		err := IncreaseUserQuota(systemAccountUserId, profitAmount, false)
		if err != nil {
			common.SysLog(fmt.Sprintf("处理OEM盈利失败: oemCode=%s, systemAccountUserId=%d, profitAmount=%d, error=%v",
				oemCode, systemAccountUserId, profitAmount, err))
			return err
		}

		// 记录OEM账户变动日志
		balanceAfter := balanceBefore + oemSubsidy
		// 将quota转换为人民币（1美金=500000quota=7.0人民币）
		profitRmb := float64(oemSubsidy) / common.QuotaPerUnit * 7.0
		accountLog := &OemAccountLog{
			OemId:          oemConfig.Id,
			OemAdminUserId: systemAccountUserId,
			CreatedAt:      common.GetTimestamp(),
			Type:           OemAccountLogTypeProfit,
			Amount:         oemSubsidy, // 正数
			BalanceBefore:  balanceBefore,
			BalanceAfter:   balanceAfter,
			RelatedLogId:   &logId,
			RelatedUserId:  &userId,
			Content:        fmt.Sprintf("OEM：+¥%.6f", profitRmb),
		}
		if err := RecordOemAccountLog(accountLog); err != nil {
			common.SysLog(fmt.Sprintf("记录OEM账户变动日志失败: error=%v", err))
		}

		common.SysLog(fmt.Sprintf("OEM盈利增加: oemCode=%s, systemAccountUserId=%d, profitAmount=%d",
			oemCode, systemAccountUserId, profitAmount))
	}

	return nil
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else if logType == LogTypeTopUpAndConsume {
		tx = LOG_DB.Where("logs.type in (1,2))")
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if username != "" {
		tx = tx.Where("logs.username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
			return logs, total, err
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("logs.user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("logs.user_id = ? and logs.type = ?", userId, logType)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	formatUserLogs(logs)
	return logs, total, err
}

func SearchAllLogs(keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("type = ? or content LIKE ?", keyword, keyword+"%").Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	return logs, err
}

func SearchUserLogs(userId int, keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("user_id = ? and type = ?", userId, keyword).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs)
	return logs, err
}

type Stat struct {
	Quota int `json:"quota"`
	Rpm   int `json:"rpm"`
	Tpm   int `json:"tpm"`
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat) {
	tx := LOG_DB.Table("logs").Select("sum(quota) quota")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, sum(prompt_tokens) + sum(completion_tokens) tpm")

	if username != "" {
		tx = tx.Where("username = ?", username)
		rpmTpmQuery = rpmTpmQuery.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name like ?", modelName)
		rpmTpmQuery = rpmTpmQuery.Where("model_name like ?", modelName)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
	}

	tx = tx.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	tx.Scan(&stat)
	rpmTpmQuery.Scan(&stat)

	return stat
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}
