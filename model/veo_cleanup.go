package model

import (
	"strconv"
	"strings"
	"time"

	"one-api/common"
)

const veoFailReasonMinSizeToClean = 8192 // 8KB，超过认为是大字段（可能为 data URI/base64）

// StartVeoFailReasonCleaner 启动清理线程：定期清理超过保留期的 Veo 成功任务的 fail_reason 大字段
func StartVeoFailReasonCleaner(retentionHours int, intervalMinutes int) {
	if retentionHours <= 0 || intervalMinutes <= 0 {
		common.SysLog("[VeoCleaner] disabled due to non-positive config")
		return
	}
	common.SysLog(
		"[VeoCleaner] enabled: retentionHours=" +
			time.Duration(retentionHours).String() +
			" interval=" + (time.Duration(intervalMinutes) * time.Minute).String(),
	)

	go func() {
		// 立即执行一次，随后按周期执行
		cleanVeoFailReasons(retentionHours)
		ticker := time.NewTicker(time.Duration(intervalMinutes) * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cleanVeoFailReasons(retentionHours)
		}
	}()
}

func cleanVeoFailReasons(retentionHours int) {
	cutoff := time.Now().Add(-time.Duration(retentionHours) * time.Hour).Unix()

	// 仅查询必要字段，避免加载大字段到内存
	var tasks []Task
	err := DB.Select("id, fail_reason, model_name, finish_time, status").
		Where("status = ? AND model_name like '%veo%' AND length(fail_reason) >1000 AND finish_time > 0 AND finish_time < ?", TaskStatusSuccess, cutoff).
		Find(&tasks).Error
	if err != nil {
		common.SysError("[VeoCleaner] query tasks failed: " + err.Error())
		return
	}

	var idsToClean []int64
	for _, t := range tasks {
		//modelLower := strings.ToLower(t.ModelName)
		//if !strings.Contains(modelLower, "veo") {
		//	continue
		//}
		fr := t.FailReason
		if fr == "" {
			continue
		}
		// 满足以下任一条件即判定需清理：
		// 1) 以 data: 开头（data URI）
		// 2) 长度超过阈值（疑似大 base64）
		if strings.HasPrefix(fr, "data:") || len(fr) >= veoFailReasonMinSizeToClean {
			idsToClean = append(idsToClean, t.ID)
		}
	}

	if len(idsToClean) == 0 {
		return
	}

	// 分批更新，避免一次 SQL 过长
	batchSize := 500
	for i := 0; i < len(idsToClean); i += batchSize {
		end := i + batchSize
		if end > len(idsToClean) {
			end = len(idsToClean)
		}
		batch := idsToClean[i:end]
		err = TaskBulkUpdateByID(batch, map[string]any{
			"fail_reason": "expired",
		})
		if err != nil {
			common.SysError("[VeoCleaner] update tasks failed: " + err.Error())
			return
		}
	}

	common.SysLog("[VeoCleaner] cleaned records: " + strconv.Itoa(len(idsToClean)))
}
