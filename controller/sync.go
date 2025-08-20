package controller

import (
	"fmt"
	"math"
	"net/http"
	"one-api/common"
	"one-api/model"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

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

	// 创建新用户
	user := &model.User{
		Id:          int64(req.Id),
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: req.Username,
		Status:      common.UserStatusEnabled,
	}

	// 调用Insert方法插入用户
	if err := user.Insert(0); err != nil {
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
	_, err = model.GetUserById(userId, true)
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
	key, err := common.GenerateKey()
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
	UserId   int64  `json:"user_id,string" binding:"required,min=1"`
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
	_, err := model.GetUserById(req.UserId, true)
	if err == nil {
		// 用户存在
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "用户存在",
		})
		return
	}

	// 用户不存在，创建新用户
	user := &model.User{
		Id:          req.UserId,
		Username:    req.UserName,
		Password:    req.UserName, // 使用user_name作为密码
		DisplayName: req.UserName,
		Status:      common.UserStatusEnabled,
	}

	// 调用Insert方法插入用户
	if err := user.Insert(0); err != nil {
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
	var userId int64
	var err error
	if userIdStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的用户ID: 请通过查询参数提供有效的user_id",
		})
		return
	} else {
		// 从查询参数解析user_id
		var parseErr error
		userId, parseErr = strconv.ParseInt(userIdStr, 10, 64)
		if parseErr != nil {
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

	// 计算人民币额度 (1 美金 = 7.3 人民币)
	const dollarToRmbRate = 7.3
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
	userId, _ := strconv.ParseInt(c.Query("user_id"), 10, 64)

	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, userId)
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
		// 计算quota_rmb (美元汇率7.3)
		extraLog.QuotaRmb = math.Round(extraLog.QuotaDollar*7.3*100) / 100
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
		user, err := model.GetUserById(int64(userId), true)
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
	user, err := model.GetUserById(req.UserId, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}

	// 计算充值的tokens数量
	// 1美元 = 500000 tokens
	// 1美元 = 7.3人民币
	const tokensPerDollar = 500000
	const dollarToRmbRate = 7.3
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
	UserId   int64   `json:"user_id" binding:"required,min=1"`
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
