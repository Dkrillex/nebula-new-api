package controller

import (
	"net/http"
	"one-api/common"
	"one-api/model"

	"github.com/gin-gonic/gin"
)

// SyncUserRequest 定义外部系统同步用户的请求结构
type SyncUserRequest struct {
	Id       int    `json:"id" binding:"required"`
	Username string `json:"username" binding:"required,max=12"`
	Password string `json:"password" binding:"required,min=8,max=20"`
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
		Id:          req.Id,
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
		c.JSON(http.StatusBadRequest, gin.H{
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

	// 获取用户ID（从系统访问令牌中提取）
	userId := c.GetInt("id")

	// 检查令牌是否存在且属于该用户
	cleanToken, err := model.GetTokenByKey(key, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的API令牌",
		})
		return
	}

	// 验证令牌是否属于当前用户
	if cleanToken.UserId != userId {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "令牌不属于该用户",
		})
		return
	}

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
