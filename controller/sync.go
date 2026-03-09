package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/middleware"
	"one-api/model"
	"one-api/types"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// getSyncUserCacheFromRequest 解析同步请求中的用户：requestUserId 在上游（如 Java）中；若在 one_api 中不存在则回退为当前认证用户（便于本地联调）
func getSyncUserCacheFromRequest(c *gin.Context, requestUserId int) (userId int, userCache *model.UserBase, newAPIError *types.NewAPIError) {
	if requestUserId <= 0 {
		if authId, ok := c.Get("id"); ok {
			if id, _ := authId.(int); id > 0 {
				requestUserId = id
			}
		}
	}
	if requestUserId <= 0 {
		return 0, nil, types.NewError(errors.New("无效的用户ID"), types.ErrorCodeInvalidRequest)
	}
	userCache, err := model.GetUserCache(requestUserId)
	if err == nil {
		return requestUserId, userCache, nil
	}
	user, dbErr := model.GetUserById(requestUserId, true)
	if dbErr == nil && user != nil {
		return requestUserId, user.ToBaseUser(), nil
	}
	// 上游 user_id 在 one_api 中不存在时，回退为当前认证用户（SYNC_ACCESS_TOKEN 对应 root），便于本地联调
	if authId, ok := c.Get("id"); ok {
		if authUserId, _ := authId.(int); authUserId > 0 && authUserId != requestUserId {
			userCache, err = model.GetUserCache(authUserId)
			if err != nil {
				user, dbErr = model.GetUserById(authUserId, true)
				if dbErr == nil && user != nil {
					return authUserId, user.ToBaseUser(), nil
				}
			} else {
				return authUserId, userCache, nil
			}
		}
	}
	return 0, nil, types.NewError(errors.New("用户不存在"), types.ErrorCodeInvalidRequest)
}

// SyncUserRequest 定义外部系统同步用户的请求结构
type SyncUserRequest struct {
	Id       int    `json:"id" binding:"required"`
	Username string `json:"username" binding:"required,max=30"`
	Password string `json:"password" binding:"required,min=8,max=30"`
}

// SyncUser 处理外部系统同步用户的请求
// @Summary 外部系统同步用户
// @Description 供外部系统调用，同步创建用户账号
// @Tags 外部系统集成
// @Accept json
// @Produce json
// @Param data body SyncUserRequest true "用户信息"
// @Success 200 {object} common.Response{data=model.User}
// @Failure 400 {object} common.Response{msg=string}
// @Failure 500 {object} common.Response{msg=string}
// @Router /api/sync/user [post]
func SyncUser(c *gin.Context) {
	var req SyncUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 检查用户是否已存在
	userExist, err := model.CheckUserExistOrDeleted(req.Username, "")
	if err != nil {
		common.SysError("检查用户存在性失败: " + err.Error())
		c.JSON(500, gin.H{
			"success": false,
			"message": "检查用户存在性失败",
		})
		return
	}
	if userExist {
		c.JSON(400, gin.H{
			"success": false,
			"message": "用户已存在",
		})
		return
	}

	// 获取OEM ID（从Context中获取）
	var oemId *int64
	if oemCode, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
		if codeStr, ok := oemCode.(string); ok && codeStr != "" {
			oemConfig := model.GetOemConfigByCode(codeStr)
			if oemConfig != nil {
				oemId = &oemConfig.Id
			}
		}
	}
	// 如果还是没有，尝试从Context直接获取OemId
	if oemId == nil {
		if id, exists := c.Get(string(constant.ContextKeyOemId)); exists {
			if idInt64, ok := id.(int64); ok {
				oemId = &idInt64
			}
		}
	}

	// 创建新用户
	user := &model.User{
		Id:          int(req.Id),
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: req.Username,
		Status:      common.UserStatusEnabled,
		OemId:       oemId, // 设置OEM ID
	}

	// 调用Insert方法插入用户
	if err := user.Insert(user.Id); err != nil {
		common.SysError("创建用户失败: " + err.Error())
		c.JSON(500, gin.H{
			"success": false,
			"message": "创建用户失败",
		})
		return
	}

	// 返回成功响应
	c.JSON(200, gin.H{
		"success": true,
		"message": "创建用户成功",
		"data":    user,
	})
}

// SyncGenerateAccessToken 创建API访问令牌
// @Summary 创建API访问令牌
// @Description 为指定用户创建API访问令牌
// @Tags 同步接口
// @Accept application/json
// @Produce application/json
// @Param body body model.Token true "令牌信息"
// @Success 200 {object} common.Response{data=string}
// @Failure 400 {object} common.Response{message=string}
// @Failure 500 {object} common.Response{message=string}
// @Router /api/sync/system/token [post]
func SyncGenerateAccessToken(c *gin.Context) {
	// 绑定请求体参数
	token := model.Token{}
	err := c.ShouldBindJSON(&token)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 获取用户ID
	userId := token.UserId
	if userId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的用户ID",
		})
		return
	}

	// 检查用户是否存在
	_, err = model.GetUserById(int(userId), true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}

	// 验证令牌名称长度
	if len(token.Name) > 30 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "令牌名称过长",
		})
		return
	}

	// 生成API令牌
	//randI := common.GetRandomInt(4)
	key, err := common.GenerateKey()
	//key, err := common.GenerateRandomKey(29 + randI)
	key = strings.ReplaceAll(key, "+", "-")
	key = strings.ReplaceAll(key, "/", "_")
	key = strings.ReplaceAll(key, "=", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "生成API令牌失败",
		})
		common.SysError("failed to generate API token key: " + err.Error())
		return
	}

	// 创建令牌记录
	cleanToken := model.Token{
		UserId:             userId,
		Name:               token.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        token.ExpiredTime,
		RemainQuota:        token.RemainQuota,
		UnlimitedQuota:     token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		AllowIps:           token.AllowIps,
		Group:              token.Group,
		Status:             common.TokenStatusEnabled,
	}

	// 保存令牌
	err = cleanToken.Insert()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "保存API令牌失败",
		})
		common.SysError("failed to insert API token: " + err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "生成API令牌成功",
		"data":    key,
	})
	return
}

// SyncGetUserInfo 查询用户信息
// @Summary 查询用户信息
// @Description 供外部系统调用，根据user_id查询用户信息
// @Tags 同步接口
// @Accept application/json
// @Produce application/json
// @Param user_id query int true "用户ID"
// @Success 200 {object} common.Response{data=object{id=int,username=string,request_count=int,quota=int,used_quota=int,quota_dollar=float64,used_quota_dollar=float64,quota_rmb=float64,used_quota_rmb=float64}}
// @Failure 400 {object} common.Response{message=string}
// @Failure 500 {object} common.Response{message=string}
// @Router /api/sync/system/user [get]
// SyncCheckUserExists 检查用户是否存在，如果不存在则创建
// @Summary 检查用户是否存在，如果不存在则创建
// @Description 供外部系统调用，检查用户是否存在，如果不存在则创建
// @Tags 同步接口
// @Accept application/json
// @Produce application/json
// @Param data body SyncCheckUserExistsRequest true "用户信息"
// @Success 200 {object} common.Response{success=bool,message=string}
// @Failure 400 {object} common.Response{success=bool,message=string}
// @Failure 500 {object} common.Response{success=bool,message=string}
// @Router /api/sync/system/user/exists [post]

// SyncCheckUserExistsRequest 定义检查用户存在性请求结构
type SyncCheckUserExistsRequest struct {
	UserId   int    `json:"user_id" binding:"required,min=1"`
	UserName string `json:"user_name" binding:"required,min=1,max=30"`
}

func SyncCheckUserExists(c *gin.Context) {
	// 绑定请求体参数
	var req SyncCheckUserExistsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 检查用户是否存在
	_, err := model.GetUserById(int(req.UserId), true)
	if err == nil {
		// 用户存在
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "用户存在",
		})
		return
	}

	// 获取OEM ID（从Context中获取）
	var oemId *int64
	if oemCode, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
		if codeStr, ok := oemCode.(string); ok && codeStr != "" {
			oemConfig := model.GetOemConfigByCode(codeStr)
			if oemConfig != nil {
				oemId = &oemConfig.Id
			}
		}
	}
	// 如果还是没有，尝试从Context直接获取OemId
	if oemId == nil {
		if id, exists := c.Get(string(constant.ContextKeyOemId)); exists {
			if idInt64, ok := id.(int64); ok {
				oemId = &idInt64
			}
		}
	}

	// 用户不存在，创建新用户
	user := &model.User{
		Id:          int(req.UserId),
		Username:    req.UserName,
		Password:    req.UserName, // 使用user_name作为密码
		DisplayName: req.UserName,
		Status:      common.UserStatusEnabled,
		OemId:       oemId, // 设置OEM ID
	}

	// 调用Insert方法插入用户
	if err := user.Insert(user.Id); err != nil {
		common.SysError("创建用户失败: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "创建用户失败: " + err.Error(),
		})
		return
	}

	// 创建成功
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "用户创建成功",
	})
}

func SyncGetUserInfo(c *gin.Context) {
	// 尝试从查询参数获取user_id
	userIdStr := c.Query("user_id")
	var userId int
	var err error
	if userIdStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的用户ID: 请通过查询参数提供有效的user_id",
		})
		return
	} else {
		// 从查询参数解析user_id
		userId, err = strconv.Atoi(userIdStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "无效的用户ID: 必须是有效的整数",
			})
			return
		}
	}

	if userId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的用户ID: 必须大于0",
		})
		return
	}

	// 查询用户信息
	user, err := model.GetUserById(userId, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}

	// 计算美元额度 (500000 tokens = 1 美金)
	const tokensPerDollar = 500000
	quotaDollar := float64(user.Quota) / float64(tokensPerDollar)
	usedQuotaDollar := float64(user.UsedQuota) / float64(tokensPerDollar)

	// 计算人民币额度 (1 美金 = 7.0 人民币)
	const dollarToRmbRate = 7.0
	quotaRmb := quotaDollar * dollarToRmbRate
	usedQuotaRmb := usedQuotaDollar * dollarToRmbRate

	// 构建响应数据
	responseData := gin.H{
		"id":                user.Id,
		"username":          user.Username,
		"request_count":     user.RequestCount,
		"quota":             user.Quota,
		"used_quota":        user.UsedQuota,
		"quota_dollar":      quotaDollar,
		"used_quota_dollar": usedQuotaDollar,
		"quota_rmb":         quotaRmb,
		"used_quota_rmb":    usedQuotaRmb,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "查询成功",
		"data":    responseData,
	})
	return
}

// SyncGetLogs 查询日志列表
// @Summary 查询日志列表
// @Description 供外部系统调用，查询日志列表
// @Tags 同步接口
// @Accept application/json
// @Produce application/json
// @Param p query int false "页码，默认1"
// @Param page_size query int false "每页条数，默认10"
// @Param type query int false "日志类型(0:未知,1:充值,2:消费,3:管理,4:系统,5:错误)"
// @Param username query string false "用户名"
// @Param token_name query string false "令牌名称"
// @Param model_name query string false "模型名称"
// @Param start_timestamp query int64 false "开始时间戳"
// @Param end_timestamp query int64 false "结束时间戳"
// @Param channel query int false "通道ID"
// @Param group query string false "分组"
// @Param user_id query int false "用户ID"
// @Success 200 {object} common.Response{data=model.PageInfo{items=[]LogWithExtra}}
// @Failure 400 {object} common.Response{message=string}
// @Failure 500 {object} common.Response{message=string}
// @Router /api/sync/system/log [get]

type LogWithExtra struct {
	model.Log
	CreateTime  string  `json:"createTime"`
	QuotaDollar float64 `json:"quota_dollar"`
	QuotaRmb    float64 `json:"quota_rmb"`
}

func SyncGetLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")

	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 转换日志列表，添加额外字段
	extraLogs := make([]*LogWithExtra, len(logs))
	for i, log := range logs {
		extraLog := &LogWithExtra{
			Log: *log,
		}
		// 格式化时间戳为yyyy-MM-dd HH:mm:ss
		extraLog.CreateTime = time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05")
		// 计算quota_dollar (500000 quota = 1美元)
		extraLog.QuotaDollar = float64(log.Quota) / 500000
		// 计算quota_rmb (美元汇率7.0)
		extraLog.QuotaRmb = math.Round(extraLog.QuotaDollar*7.0*100) / 100
		extraLogs[i] = extraLog
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(extraLogs)
	common.ApiSuccess(c, pageInfo)
	return
}

// SyncGetTokenLogsStat 查询API令牌使用日志统计
// @Summary 查询API令牌使用日志统计
// @Description 供外部系统调用，查询API令牌的使用日志统计
// @Tags 同步接口
// @Accept application/json
// @Produce application/json
// @Param key query string true "API令牌"
// @Param user_id query int false "用户ID"
// @Param type query int false "日志类型(0:未知,1:充值,2:消费,3:管理,4:系统,5:错误)"
// @Param start_timestamp query int64 false "开始时间戳"
// @Param end_timestamp query int64 false "结束时间戳"
// @Param model_name query string false "模型名称"
// @Param channel query int false "通道ID"
// @Param group query string false "分组"
// @Success 200 {object} common.Response{data=model.Stat}
// @Failure 400 {object} common.Response{message=string}
// @Failure 500 {object} common.Response{message=string}
// @Router /api/sync/system/log/stat [get]
func SyncGetTokenLogsStat(c *gin.Context) {
	//// 获取API令牌
	//key := c.Query("key")
	//if key == "" {
	//	c.JSON(http.StatusBadRequest, gin.H{
	//		"success": false,
	//		"message": "未提供API令牌",
	//	})
	//	return
	//}

	//// 验证令牌
	//cleanKey := strings.TrimPrefix(key, "sk-")
	//token, err := model.GetTokenByKey(cleanKey, false)
	//if err != nil {
	//	c.JSON(http.StatusBadRequest, gin.H{
	//		"success": false,
	//		"message": "无效的API令牌",
	//	})
	//	return
	//}

	// 获取查询参数
	userId, _ := strconv.Atoi(c.Query("user_id"))
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")

	// 查询用户名（如果提供了user_id）
	username := ""
	if userId > 0 {
		user, err := model.GetUserById(int(userId), true)
		if err == nil {
			username = user.Username
		}
	}

	// 查询日志统计
	stat := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, "", channel, group)

	// 返回结果
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "查询成功",
		"data":    stat,
	})
}

// SyncUpdateUserQuota 更新用户配额
// @Summary 更新用户配额
// @Description 供外部系统调用，根据人民币金额更新用户配额
// @Tags 同步接口
// @Accept application/json
// @Produce application/json
// @Param data body SyncUpdateUserQuotaRequest true "更新信息"
// @Success 200 {object} common.Response{data=model.User}
// @Failure 400 {object} common.Response{message=string}
// @Failure 500 {object} common.Response{message=string}
// @Router /api/sync/system/user/quota [post]
func SyncUpdateUserQuota(c *gin.Context) {
	var req SyncUpdateUserQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 查找用户
	user, err := model.GetUserById(int(req.UserId), true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}

	// 计算充值的tokens数量
	// 1美元 = 500000 tokens
	// 1美元 = 7.0人民币
	const tokensPerDollar = 500000
	const dollarToRmbRate = 7.0
	dollars := req.QuotaRmb / dollarToRmbRate
	addedTokens := int(dollars * float64(tokensPerDollar))

	// 更新用户配额
	user.Quota += addedTokens

	// 保存更新后的用户信息
	if err := model.DB.Save(user).Error; err != nil {
		common.SysError("更新用户配额失败: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "更新用户配额失败",
		})
		return
	}

	// 记录充值日志
	log := &model.Log{
		UserId:    user.Id,
		Username:  user.Username,
		Type:      model.LogTypeTopup,
		Content:   fmt.Sprintf("充值 %.00f 人民币，获得 %d tokens", req.QuotaRmb, addedTokens),
		Quota:     addedTokens,
		ModelName: "system",
		Ip:        c.ClientIP(),
		CreatedAt: common.GetTimestamp(),
	}
	if err := model.LOG_DB.Create(log).Error; err != nil {
		common.SysError("记录充值日志失败: " + err.Error())
		// 日志记录失败不影响主流程
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("充值成功，增加 %d tokens，当前总配额: %d", addedTokens, user.Quota),
		"data":    user,
	})
	return
}

type SyncUpdateUserQuotaRequest struct {
	UserId   int     `json:"user_id" binding:"required,min=1"`
	QuotaRmb float64 `json:"quota_rmb" binding:"required,min=0.01"`
}

// SyncUpdateTokenStatus 更新API令牌状态
// @Summary 更新API令牌状态
// @Description 供外部系统调用，更新API令牌的状态
// @Tags 同步接口
// @Accept application/json
// @Produce application/json
// @Param key query string true "API令牌"
// @Param status_only query string false "是否只更新状态"
// @Param body body model.Token true "令牌信息"
// @Success 200 {object} common.Response{message=string}
// @Failure 400 {object} common.Response{message=string}
// @Failure 500 {object} common.Response{message=string}
// @Router /api/sync/system/token/update [post]
func SyncUpdateTokenStatus(c *gin.Context) {
	// 获取API令牌
	key := c.Query("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "未提供API令牌",
		})
		return
	}
	key = strings.TrimPrefix(key, "sk-")

	// 获取status_only参数
	statusOnly := c.Query("status_only")

	// 绑定请求体参数
	token := model.Token{}
	err := c.ShouldBindJSON(&token)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 检查令牌是否存在且属于该用户
	cleanToken, err := model.GetTokenByKey(key, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的API令牌",
		})
		return
	}
	// 获取用户ID（从系统访问令牌中提取）
	//userId := c.GetInt64("id")
	//// 验证令牌是否属于当前用户
	//if cleanToken.UserId != userId {
	//	c.JSON(http.StatusBadRequest, gin.H{
	//		"success": false,
	//		"message": "令牌不属于该用户",
	//	})
	//	return
	//}

	// 如果启用令牌，检查是否过期或额度用尽
	if token.Status == common.TokenStatusEnabled {
		if cleanToken.Status == common.TokenStatusExpired && cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "令牌已过期，无法启用，请先修改令牌过期时间，或者设置为永不过期",
			})
			return
		}
		if cleanToken.Status == common.TokenStatusExhausted && cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "令牌可用额度已用尽，无法启用，请先修改令牌剩余额度，或者设置为无限额度",
			})
			return
		}
	}

	// 根据status_only参数决定更新内容
	if statusOnly != "" {
		// 只更新状态
		cleanToken.Status = token.Status
	} else {
		// 更新所有可编辑字段
		if len(token.Name) > 30 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "令牌名称过长",
			})
			return
		}
		cleanToken.Name = token.Name
		cleanToken.ExpiredTime = token.ExpiredTime
		cleanToken.RemainQuota = token.RemainQuota
		cleanToken.UnlimitedQuota = token.UnlimitedQuota
		cleanToken.ModelLimitsEnabled = token.ModelLimitsEnabled
		cleanToken.ModelLimits = token.ModelLimits
		cleanToken.AllowIps = token.AllowIps
		cleanToken.Group = token.Group
		cleanToken.Status = token.Status
	}

	// 更新令牌
	err = cleanToken.Update()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "更新令牌失败",
		})
		common.SysError("failed to update API token: " + err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新令牌成功",
	})
	return
}

// SyncPlayground 处理外部系统操练场大模型对话请求
// @Summary 外部系统操练场大模型对话
// @Description 供外部系统调用，通过请求体中的user_id指定实际扣费用户进行大模型对话
// @Tags 外部系统集成
// @Accept json
// @Produce json
// @Param data body dto.SyncPlaygroundRequest true "操练场对话请求"
// @Success 200 {object} common.Response{data=object}
// @Failure 400 {object} common.Response{msg=string}
// @Failure 500 {object} common.Response{msg=string}
// @Router /api/sync/system/pg/chat/completions [post]
func SyncPlayground(c *gin.Context) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			// 记录错误日志
			userId := c.GetInt("id")
			errorMsg := fmt.Sprintf("[SyncPlayground] user_id: %d, status: %d, error: %s", userId, newAPIError.StatusCode, newAPIError.Error())
			logger.LogError(c.Request.Context(), errorMsg)
			openAIErr := newAPIError.ToOpenAIError()
			errorResponse := gin.H{
				"error": openAIErr,
			}
			// 记录实际返回的错误响应内容，用于调试
			if newAPIError.StatusCode >= 400 {
				errorJson, _ := json.Marshal(errorResponse)
				logger.LogError(c.Request.Context(), fmt.Sprintf("[SyncPlayground] 返回错误响应 (status: %d): %s", newAPIError.StatusCode, string(errorJson)))
			}
			c.JSON(newAPIError.StatusCode, errorResponse)
		}
	}()

	// 解析请求体
	playgroundRequest := &dto.SyncPlaygroundRequest{}
	err := common.UnmarshalBodyReusable(c, playgroundRequest)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}

	// 获取用户ID
	userId := playgroundRequest.UserId
	if userId <= 0 {
		newAPIError = types.NewError(errors.New("无效的用户ID"), types.ErrorCodeInvalidRequest)
		return
	}

	// 先尝试从缓存获取用户信息
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		// 缓存中没有，从数据库查询
		user, dbErr := model.GetUserById(userId, true)
		if dbErr != nil {
			newAPIError = types.NewError(errors.New("用户不存在"), types.ErrorCodeInvalidRequest)
			return
		}
		// 将查询结果转换为缓存对象
		userCache = user.ToBaseUser()
	}

	// 检查用户状态
	if userCache.Status != common.UserStatusEnabled {
		newAPIError = types.NewError(errors.New("用户已被禁用"), types.ErrorCodeInvalidRequest)
		return
	}

	// 验证模型参数
	if playgroundRequest.Model == "" {
		newAPIError = types.NewError(errors.New("请选择模型"), types.ErrorCodeInvalidRequest)
		return
	}
	c.Set("original_model", playgroundRequest.Model)

	// 设置分组
	group := playgroundRequest.Group
	if group == "" {
		group = userCache.Group
	}
	c.Set("group", group)

	// 设置用户ID到上下文
	c.Set("id", userId)

	// 写入用户缓存到上下文
	userCache.WriteContext(c)

	// 创建临时令牌
	tempToken := &model.Token{
		UserId: int(userId),
		Name:   fmt.Sprintf("nebula-playground-%s", group),
		Group:  group,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	// 直接调用CacheGetRandomSatisfiedChannel获取最高优先级渠道，绕过getChannel的上下文依赖
	channel, _, err := model.CacheGetRandomSatisfiedChannel(c, group, playgroundRequest.Model, 0)
	if err != nil {
		newAPIError = types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败: %s", group, playgroundRequest.Model, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return
	}
	if channel == nil {
		newAPIError = types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在", group, playgroundRequest.Model), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return
	}
	// 设置渠道上下文
	newAPIError = middleware.SetupContextForSelectedChannel(c, channel, playgroundRequest.Model)
	if newAPIError != nil {
		return
	}

	// 设置请求开始时间
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())

	// 转发请求到模型
	Relay(c, types.RelayFormatOpenAI)
}

// SyncImageGeneration 处理外部系统图片生成请求
// @Summary 外部系统图片生成
// @Description 供外部系统调用，通过请求体中的user_id指定实际扣费用户进行图片生成
// @Tags 外部系统集成
// @Accept json
// @Produce json
// @Param data body dto.SyncImageGenerationRequest true "图片生成请求"
// @Success 200 {object} common.Response{data=object}
// @Failure 400 {object} common.Response{msg=string}
// @Failure 500 {object} common.Response{msg=string}
// @Router /api/sync/system/images/generations [post]
func SyncImageGeneration(c *gin.Context) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	// 解析请求体
	imageRequest := &dto.SyncImageGenerationRequest{}
	err := common.UnmarshalBodyReusable(c, imageRequest)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}

	userId, userCache, newAPIError := getSyncUserCacheFromRequest(c, imageRequest.UserId)
	if newAPIError != nil {
		return
	}

	// 检查用户状态
	if userCache.Status != common.UserStatusEnabled {
		newAPIError = types.NewError(errors.New("用户已被禁用"), types.ErrorCodeInvalidRequest)
		return
	}

	// 验证模型参数
	if imageRequest.Model == "" {
		newAPIError = types.NewError(errors.New("请选择模型"), types.ErrorCodeInvalidRequest)
		return
	}
	c.Set("original_model", imageRequest.Model)

	// 设置分组
	group := imageRequest.Group
	if group == "" {
		group = userCache.Group
	}
	c.Set("group", group)

	// 设置用户ID到上下文
	c.Set("id", userId)

	// 写入用户缓存到上下文
	userCache.WriteContext(c)

	// 创建临时令牌
	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("nebula-image-generations-%s", group),
		Group:  group,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	// 直接调用CacheGetRandomSatisfiedChannel获取最高优先级渠道，绕过getChannel的上下文依赖
	channel, _, err := model.CacheGetRandomSatisfiedChannel(c, group, imageRequest.Model, 0)
	if err != nil {
		newAPIError = types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败: %s", group, imageRequest.Model, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return
	}
	if channel == nil {
		newAPIError = types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在", group, imageRequest.Model), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return
	}
	// 设置渠道上下文
	newAPIError = middleware.SetupContextForSelectedChannel(c, channel, imageRequest.Model)
	if newAPIError != nil {
		return
	}

	// 构建标准的ImageRequest，包含Extra字段
	standardImageRequest := &dto.ImageRequest{
		Model:          imageRequest.Model,
		Prompt:         imageRequest.Prompt,
		N:              imageRequest.N,
		Size:           imageRequest.Size,
		Quality:        imageRequest.Quality,
		ResponseFormat: imageRequest.ResponseFormat,
		Extra:          imageRequest.Extra, // 传递私有参数
	}

	// 如果有Style字段，转换为json.RawMessage
	if imageRequest.Style != "" {
		styleBytes, _ := json.Marshal(imageRequest.Style)
		standardImageRequest.Style = styleBytes
	}

	// 将标准请求重新设置到请求体中
	requestBytes, err := json.Marshal(standardImageRequest)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(requestBytes))
	c.Request.ContentLength = int64(len(requestBytes))

	// 设置请求开始时间
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())

	// 转发请求到模型
	Relay(c, types.RelayFormatOpenAIImage)
}

// SyncVideoGeneration 处理外部系统视频生成请求
// @Summary 外部系统视频生成
// @Description 供外部系统调用，通过请求体中的user_id指定实际扣费用户进行视频生成
// @Tags 外部系统集成
// @Accept json
// @Produce json
// @Param data body dto.SyncVideoGenerationRequest true "视频生成请求"
// @Success 200 {object} common.Response{data=object}
// @Failure 400 {object} common.Response{msg=string}
// @Failure 500 {object} common.Response{msg=string}
// @Router /api/sync/system/videos/generations [post]
func SyncVideoGeneration(c *gin.Context) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	// 解析请求体
	videoRequest := &dto.SyncVideoGenerationRequest{}
	err := common.UnmarshalBodyReusable(c, videoRequest)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}

	// 获取用户ID
	userId := videoRequest.UserId
	if userId <= 0 {
		newAPIError = types.NewError(errors.New("无效的用户ID"), types.ErrorCodeInvalidRequest)
		return
	}

	// 先尝试从缓存获取用户信息
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		// 缓存中没有，从数据库查询
		user, dbErr := model.GetUserById(userId, true)
		if dbErr != nil {
			newAPIError = types.NewError(errors.New("用户不存在"), types.ErrorCodeInvalidRequest)
			return
		}
		// 将查询结果转换为缓存对象
		userCache = user.ToBaseUser()
	}

	// 检查用户状态
	if userCache.Status != common.UserStatusEnabled {
		newAPIError = types.NewError(errors.New("用户已被禁用"), types.ErrorCodeInvalidRequest)
		return
	}

	// 验证模型参数
	if videoRequest.Model == "" {
		newAPIError = types.NewError(errors.New("请选择模型"), types.ErrorCodeInvalidRequest)
		return
	}
	c.Set("original_model", videoRequest.Model)

	// 设置分组
	group := videoRequest.Group
	if group == "" {
		group = userCache.Group
	}
	c.Set("group", group)

	// 设置用户ID到上下文
	c.Set("id", userId)

	// 写入用户缓存到上下文
	userCache.WriteContext(c)

	// 创建临时令牌
	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("nebula-video-generations-%s", group),
		Group:  group,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	// 直接调用CacheGetRandomSatisfiedChannel获取最高优先级渠道，绕过getChannel的上下文依赖
	channel, _, err := model.CacheGetRandomSatisfiedChannel(c, group, videoRequest.Model, 0)
	if err != nil {
		newAPIError = types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败: %s", group, videoRequest.Model, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return
	}
	if channel == nil {
		newAPIError = types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在", group, videoRequest.Model), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return
	}
	// 设置渠道上下文
	newAPIError = middleware.SetupContextForSelectedChannel(c, channel, videoRequest.Model)
	if newAPIError != nil {
		return
	}

	// 转换为统一视频生成请求格式
	standardVideoRequest := dto.VideoRequest{
		Model:          videoRequest.Model,
		Prompt:         videoRequest.Prompt,
		Image:          videoRequest.Image,
		Duration:       videoRequest.Duration,
		Width:          videoRequest.Width,
		Height:         videoRequest.Height,
		Fps:            videoRequest.Fps,
		Seed:           videoRequest.Seed,
		N:              int(videoRequest.N),
		ResponseFormat: videoRequest.ResponseFormat,
		User:           fmt.Sprintf("user-%d", userCache.Id),
	}

	// 处理额外参数
	if len(videoRequest.Extra) > 0 {
		metadata := make(map[string]interface{})
		for k, v := range videoRequest.Extra {
			var value interface{}
			if err := json.Unmarshal(v, &value); err == nil {
				metadata[k] = value
			}
		}
		standardVideoRequest.Metadata = metadata
	}

	// 将标准请求重新设置到请求体中
	requestBytes, err := json.Marshal(standardVideoRequest)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(requestBytes))
	c.Request.ContentLength = int64(len(requestBytes))

	// 修改请求路径为标准视频生成路径
	c.Request.URL.Path = "/v1/video/generations"

	// 设置请求开始时间
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())

	// 转发请求到模型
	RelayTask(c)
}

// SyncGetVideoTask 处理外部系统视频任务状态查询请求
// @Summary 外部系统视频任务状态查询
// @Description 供外部系统调用，查询视频生成任务状态
// @Tags 外部系统集成
// @Accept json
// @Produce json
// @Param task_id path string true "任务ID"
// @Param user_id query int true "用户ID"
// @Success 200 {object} common.Response{data=object}
// @Failure 400 {object} common.Response{msg=string}
// @Failure 500 {object} common.Response{msg=string}
// @Router /api/sync/system/videos/generations [get]
func SyncGetVideoTask(c *gin.Context) {
	//common.SysLog(fmt.Sprintf("[SyncGetVideoTask] 请求路径: %s", c.Request.URL.Path))

	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	// 获取任务ID
	taskId := c.Param("task_id")
	if taskId == "" {
		common.SysError("[SyncGetVideoTask] 任务ID为空")
		newAPIError = types.NewError(errors.New("任务ID不能为空"), types.ErrorCodeInvalidRequest)
		return
	}

	// 获取用户ID（从URL参数）
	userIdStr := c.Query("user_id")
	if userIdStr == "" {
		common.SysError("[SyncGetVideoTask] 用户ID为空")
		newAPIError = types.NewError(errors.New("用户ID不能为空"), types.ErrorCodeInvalidRequest)
		return
	}

	userId, err := strconv.Atoi(userIdStr)
	if err != nil || userId <= 0 {
		newAPIError = types.NewError(errors.New("无效的用户ID"), types.ErrorCodeInvalidRequest)
		return
	}

	// 先尝试从缓存获取用户信息
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		// 缓存中没有，从数据库查询
		user, dbErr := model.GetUserById(userId, true)
		if dbErr != nil {
			newAPIError = types.NewError(errors.New("用户不存在"), types.ErrorCodeInvalidRequest)
			return
		}
		// 将查询结果转换为缓存对象
		userCache = user.ToBaseUser()
	}

	// 检查用户状态
	if userCache.Status != common.UserStatusEnabled {
		newAPIError = types.NewError(errors.New("用户已被禁用"), types.ErrorCodeInvalidRequest)
		return
	}

	// 设置分组
	group := userCache.Group
	c.Set("group", group)

	// 设置用户ID到上下文
	c.Set("id", userId)

	// 写入用户缓存到上下文
	userCache.WriteContext(c)

	// 创建临时令牌
	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("nebula-video-task-query-%s", group),
		Group:  group,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	// 修改请求路径为标准视频任务查询路径
	newPath := "/v1/video/generations/" + taskId
	c.Request.URL.Path = newPath

	// 设置请求开始时间
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())

	// 转发请求到视频任务查询接口
	RelayTask(c)
}

// SyncDownloadVideoBase64 外部系统下载视频（base64）
// @Summary 外部系统下载视频（base64）
// @Description 供外部系统调用，传入 user_id、task_id、generation_id 下载视频并返回 base64
// @Tags 外部系统集成
// @Accept json
// @Produce json
// @Param task_id path string true "任务ID"
// @Param gen_id path string true "生成ID"
// @Param user_id query int true "用户ID"
// @Success 200 {object} common.Response{data=object}
// @Failure 400 {object} common.Response{msg=string}
// @Failure 500 {object} common.Response{msg=string}
// @Router /api/sync/system/videos/generations/{task_id}/download/{gen_id} [get]
func SyncDownloadVideoBase64(c *gin.Context) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	// 获取查询参数（统一简洁接口）
	videoId := c.Query("id")
	if videoId == "" {
		newAPIError = types.NewError(errors.New("id 参数不能为空"), types.ErrorCodeInvalidRequest)
		return
	}

	// 获取用户ID
	userIdStr := c.Query("user_id")
	if userIdStr == "" {
		newAPIError = types.NewError(errors.New("用户ID不能为空"), types.ErrorCodeInvalidRequest)
		return
	}
	userId, err := strconv.Atoi(userIdStr)
	if err != nil || userId <= 0 {
		newAPIError = types.NewError(errors.New("无效的用户ID"), types.ErrorCodeInvalidRequest)
		return
	}

	// 校验并写入用户上下文（与 SyncGetVideoTask 同逻辑）
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		user, dbErr := model.GetUserById(userId, true)
		if dbErr != nil {
			newAPIError = types.NewError(errors.New("用户不存在"), types.ErrorCodeInvalidRequest)
			return
		}
		userCache = user.ToBaseUser()
	}
	if userCache.Status != common.UserStatusEnabled {
		newAPIError = types.NewError(errors.New("用户已被禁用"), types.ErrorCodeInvalidRequest)
		return
	}
	group := userCache.Group
	c.Set("group", group)
	c.Set("id", userId)
	userCache.WriteContext(c)

	// 创建临时令牌并注入上下文（用于后续查任务和鉴权）
	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("nebula-video-download-%s", group),
		Group:  group,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	// 直接复用内部下载逻辑：路径转换并转发到内部受保护下载接口
	originalPath := c.Request.URL.Path
	newPath := "/v1/video/generations/download"
	c.Request.URL.Path = newPath
	common.SysLog(fmt.Sprintf("[SyncDownloadVideoBase64] 路径转换: %s -> %s (id=%s)", originalPath, newPath, videoId))

	// 设置请求开始时间
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())

	// 转发到内部处理
	VideoDownloadBase64(c)
}

// SyncImageEdits 处理系统间的图像编辑请求
// 支持 JSON 和 multipart/form-data 两种格式
func SyncImageEdits(c *gin.Context) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	// 解析请求体
	imageRequest := &dto.SyncImageGenerationRequest{}
	err := common.UnmarshalBodyReusable(c, imageRequest)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}

	// 获取用户ID
	userId := imageRequest.UserId
	if userId <= 0 {
		newAPIError = types.NewError(errors.New("无效的用户ID"), types.ErrorCodeInvalidRequest)
		return
	}

	// 先尝试从缓存获取用户信息
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		// 缓存中没有，从数据库查询
		user, dbErr := model.GetUserById(userId, true)
		if dbErr != nil {
			newAPIError = types.NewError(errors.New("用户不存在"), types.ErrorCodeInvalidRequest)
			return
		}
		// 将查询结果转换为缓存对象
		userCache = user.ToBaseUser()
	}

	// 检查用户状态
	if userCache.Status != common.UserStatusEnabled {
		newAPIError = types.NewError(errors.New("用户已被禁用"), types.ErrorCodeInvalidRequest)
		return
	}

	// 验证模型参数
	if imageRequest.Model == "" {
		newAPIError = types.NewError(errors.New("请选择模型"), types.ErrorCodeInvalidRequest)
		return
	}
	c.Set("original_model", imageRequest.Model)

	// 设置分组
	group := imageRequest.Group
	if group == "" {
		group = userCache.Group
	}
	c.Set("group", group)

	// 设置用户ID到上下文
	c.Set("id", userId)

	// 写入用户缓存到上下文
	userCache.WriteContext(c)

	// 创建临时令牌
	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("nebula-image-edits-%s", group),
		Group:  group,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	// 直接调用CacheGetRandomSatisfiedChannel获取最高优先级渠道，绕过getChannel的上下文依赖
	channel, _, err := model.CacheGetRandomSatisfiedChannel(c, group, imageRequest.Model, 0)
	if err != nil {
		newAPIError = types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败: %s", group, imageRequest.Model, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return
	}
	if channel == nil {
		newAPIError = types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在", group, imageRequest.Model), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return
	}
	// 设置渠道上下文
	newAPIError = middleware.SetupContextForSelectedChannel(c, channel, imageRequest.Model)
	if newAPIError != nil {
		return
	}

	// 构建标准的ImageRequest，包含Extra字段
	standardImageRequest := &dto.ImageRequest{
		Model:          imageRequest.Model,
		Prompt:         imageRequest.Prompt,
		N:              imageRequest.N,
		Size:           imageRequest.Size,
		Quality:        imageRequest.Quality,
		ResponseFormat: imageRequest.ResponseFormat,
		InputFidelity:  imageRequest.InputFidelity, // 图生图特有参数
		Extra:          imageRequest.Extra,         // 传递私有参数（包括 image）
	}

	// 调试日志：简化输出接收到的入参
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("[SyncImageEdits] 接收参数: Model=%s, Prompt=%s, Size=%s, Quality=%s, N=%d, InputFidelity=%s, UserId=%d",
			imageRequest.Model, imageRequest.Prompt, imageRequest.Size, imageRequest.Quality, imageRequest.N, imageRequest.InputFidelity, userId))
		if standardImageRequest.Extra != nil {
			if _, hasImage := standardImageRequest.Extra["image"]; hasImage {
				common.SysLog(fmt.Sprintf("[SyncImageEdits] 输入图片: 1张"))
			} else if imagesData, ok := standardImageRequest.Extra["images"]; ok {
				var images []string
				if json.Unmarshal(imagesData, &images) == nil {
					common.SysLog(fmt.Sprintf("[SyncImageEdits] 输入图片: %d张", len(images)))
				}
			}
		}
	}

	// 如果有Style字段，转换为json.RawMessage
	if imageRequest.Style != "" {
		styleBytes, _ := json.Marshal(imageRequest.Style)
		standardImageRequest.Style = styleBytes
	}

	// 将标准请求重新设置到请求体中
	requestBytes, err := json.Marshal(standardImageRequest)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}

	// 清除可能存在的 MultipartForm（避免适配器误判为 multipart 格式）
	c.Request.MultipartForm = nil
	c.Request.PostForm = nil

	// 重新设置请求体
	c.Request.Body = io.NopCloser(bytes.NewReader(requestBytes))
	c.Request.ContentLength = int64(len(requestBytes))

	// 显式设置 Content-Type 为 application/json（确保适配器正确识别为 JSON 格式）
	c.Request.Header.Set("Content-Type", "application/json")

	// 设置请求开始时间
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())

	// 转发请求到模型
	Relay(c, types.RelayFormatOpenAIImage)
}

// SyncRealtime 处理外部系统实时对话请求
// @Summary 外部系统实时对话
// @Description 供外部系统调用，通过查询参数中的user_id指定实际扣费用户进行实时对话（WebSocket连接）
// @Tags 外部系统集成
// @Accept json
// @Produce json
// @Param user_id query int true "用户ID"
// @Param model query string true "模型名称"
// @Param group query string false "分组名称"
// @Success 200 {object} common.Response{data=object}
// @Failure 400 {object} common.Response{msg=string}
// @Failure 500 {object} common.Response{msg=string}
// @Router /api/sync/system/realtime [get]
func SyncRealtime(c *gin.Context) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	// 从查询参数中解析请求
	realtimeRequest := &dto.SyncRealtimeRequest{}
	if err := c.ShouldBindQuery(realtimeRequest); err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}

	// 验证必需参数（Go系统只需要 nebula_api_id，它就是这个系统的 userId）
	if realtimeRequest.NebulaApiId <= 0 {
		newAPIError = types.NewError(errors.New("无效的Nebula API ID"), types.ErrorCodeInvalidRequest)
		return
	}

	// 使用 nebula_api_id 作为 Go 系统的用户ID（对于Go系统来说，nebula_api_id 就是 userId）
	userId := realtimeRequest.NebulaApiId

	// 先尝试从缓存获取用户信息
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		// 缓存中没有，从数据库查询
		user, dbErr := model.GetUserById(userId, true)
		if dbErr != nil {
			newAPIError = types.NewError(errors.New("用户不存在"), types.ErrorCodeInvalidRequest)
			return
		}
		// 将查询结果转换为缓存对象
		userCache = user.ToBaseUser()
	}

	// 检查用户状态
	if userCache.Status != common.UserStatusEnabled {
		newAPIError = types.NewError(errors.New("用户已被禁用"), types.ErrorCodeInvalidRequest)
		return
	}

	// 验证模型参数
	if realtimeRequest.Model == "" {
		newAPIError = types.NewError(errors.New("请选择模型"), types.ErrorCodeInvalidRequest)
		return
	}
	c.Set("original_model", realtimeRequest.Model)

	// 设置分组
	group := realtimeRequest.Group
	if group == "" {
		group = userCache.Group
	}
	c.Set("group", group)

	// 设置用户ID到上下文（nebula_api_id 就是 Go 系统的 userId）
	c.Set("id", userId)

	// 写入用户缓存到上下文
	userCache.WriteContext(c)

	// 创建临时令牌
	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("nebula-realtime-%s", group),
		Group:  group,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	// 直接调用CacheGetRandomSatisfiedChannel获取最高优先级渠道
	channel, _, err := model.CacheGetRandomSatisfiedChannel(c, group, realtimeRequest.Model, 0)
	if err != nil {
		newAPIError = types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败: %s", group, realtimeRequest.Model, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return
	}
	if channel == nil {
		newAPIError = types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在", group, realtimeRequest.Model), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		return
	}
	// 设置渠道上下文
	newAPIError = middleware.SetupContextForSelectedChannel(c, channel, realtimeRequest.Model)
	if newAPIError != nil {
		return
	}

	// 保存原始路径信息，用于 genBaseRelayInfo 中判断是否为 sync realtime 请求
	// 注意：需要在修改路径之前保存，以便 genBaseRelayInfo 能够正确识别
	c.Set("is_sync_realtime", true)

	// 将请求路径修改为 /v1/realtime，保留查询参数中的 model
	c.Request.URL.Path = "/v1/realtime"
	// 确保查询参数中包含 model
	if c.Request.URL.RawQuery == "" {
		c.Request.URL.RawQuery = fmt.Sprintf("model=%s", realtimeRequest.Model)
	} else {
		// 如果已经有查询参数，检查是否已有 model 参数
		if !strings.Contains(c.Request.URL.RawQuery, "model=") {
			c.Request.URL.RawQuery += fmt.Sprintf("&model=%s", realtimeRequest.Model)
		}
	}

	// 设置请求开始时间
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())

	// 转发请求到实时对话模型（WebSocket）
	Relay(c, types.RelayFormatOpenAIRealtime)
}
