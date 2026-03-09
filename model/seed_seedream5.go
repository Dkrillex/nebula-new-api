package model

import (
	"os"
	"strconv"
	"time"

	"one-api/common"
	"one-api/constant"
)

const seedream5Model = "doubao-seedream-5-0-260128"
const seedream5Group = "default"

// SeedSeedream5ChannelIfNeeded 本地测试：若设置 SEEDREAM5_ARK_KEY 且库中尚无该模型渠道，则自动插入渠道与能力（仅测试用，提交时可删）
func SeedSeedream5ChannelIfNeeded() {
	key := os.Getenv("SEEDREAM5_ARK_KEY")
	if key == "" {
		return
	}
	var n int64
	DB.Model(&Ability{}).Where("`group` = ? AND model = ? AND enabled = ?", seedream5Group, seedream5Model, true).Count(&n)
	if n > 0 {
		return
	}
	now := time.Now().Unix()
	ch := &Channel{
		Type:        constant.ChannelTypeVolcEngine,
		Key:         key,
		Name:        "Seedream 5.0 本地测试",
		Status:      common.ChannelStatusEnabled,
		CreatedTime: now,
		TestTime:    0,
		Group:       seedream5Group,
		Models:      seedream5Model,
	}
	if err := DB.Create(ch).Error; err != nil {
		common.SysLog("SeedSeedream5ChannelIfNeeded create channel failed: " + err.Error())
		return
	}
	priority := int64(0)
	weight := uint(0)
	a := &Ability{
		Group:     seedream5Group,
		Model:     seedream5Model,
		ChannelId: ch.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}
	if err := DB.Create(a).Error; err != nil {
		common.SysLog("SeedSeedream5ChannelIfNeeded create ability failed: " + err.Error())
		return
	}
	common.SysLog("SeedSeedream5ChannelIfNeeded: 已插入渠道与能力, channel_id=" + strconv.Itoa(ch.Id))
}
