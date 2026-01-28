package types

import (
	"fmt"
	"one-api/setting/ratio_setting"
)

type GroupRatioInfo struct {
	GroupRatio        float64
	GroupSpecialRatio float64
	HasSpecialRatio   bool
}

type PriceData struct {
	ModelPrice                  float64
	ModelRatio                  float64
	OfficialModelPrice          float64 // 原始模型价格（应用OEM折扣前）
	OfficialModelRatio          float64 // 原始模型倍率（应用OEM折扣前）
	OemModelPrice               float64 // OEM平台模型价格（原厂价格 * OEM折扣）
	OemModelRatio               float64 // OEM平台模型倍率（原厂倍率 * OEM折扣）
	CompletionRatio             float64
	CacheRatio                  float64
	CacheCreationRatio          float64
	ImageRatio                  float64 // 用户使用的图片倍率（已应用OEM用户折扣）
	OfficialImageRatio          float64 // 原始图片倍率（应用OEM折扣前）
	OemImageRatio               float64 // OEM平台图片倍率（原厂倍率 * OEM折扣）
	AudioRatio                  float64 // 用户使用的音频倍率（已应用OEM用户折扣）
	OfficialAudioRatio          float64 // 原始音频倍率（应用OEM折扣前）
	OemAudioRatio               float64 // OEM平台音频倍率（原厂倍率 * OEM折扣）
	AudioCompletionRatio        float64
	ImageCompletionRatio        float64
	VideoPricePerSecond         float64 // 用户使用的视频每秒价格（已应用OEM用户折扣）
	OfficialVideoPricePerSecond float64 // 原始视频每秒价格（应用OEM折扣前）
	OemVideoPricePerSecond      float64 // OEM平台视频每秒价格（原厂价格 * OEM折扣）
	ImagePricePerImage          float64 // 用户使用的图片每张价格（已应用OEM用户折扣）
	OfficialImagePricePerImage  float64 // 原始图片每张价格（应用OEM折扣前）
	OemImagePricePerImage       float64 // OEM平台图片每张价格（原厂价格 * OEM折扣）
	OtherRatios                 map[string]float64
	UsePrice                    bool
	UseImageTokenPricing        bool                            // 是否使用图像Token表定价
	ImageTokenPricing           ratio_setting.ImageTokenPricing // 图像Token表定价配置
	ShouldPreConsumedQuota      int
	GroupRatioInfo              GroupRatioInfo
}

type PerCallPriceData struct {
	ModelPrice                 float64
	OfficialModelPrice         float64 // 原始模型价格（应用OEM折扣前）
	OemModelPrice              float64 // OEM平台模型价格（原厂价格 * OEM折扣）
	OfficialImagePricePerImage float64 // 原始图片每张价格（应用OEM折扣前）
	OemImagePricePerImage      float64 // OEM平台图片每张价格（原厂价格 * OEM折扣）
	Quota                      int
	GroupRatioInfo             GroupRatioInfo
}

func (p PriceData) ToSetting() string {
	if p.UseImageTokenPricing {
		return fmt.Sprintf("UseImageTokenPricing: true, ImageTokenPricing: {InputTextPrice: %f, InputImagePrice: %f, OutputImagePrice: %f}, GroupRatio: %f",
			p.ImageTokenPricing.InputTextPrice, p.ImageTokenPricing.InputImagePrice, p.ImageTokenPricing.OutputImagePrice, p.GroupRatioInfo.GroupRatio)
	}
	return fmt.Sprintf("ModelPrice: %f, ModelRatio: %f, CompletionRatio: %f, CacheRatio: %f, GroupRatio: %f, UsePrice: %t, CacheCreationRatio: %f, ShouldPreConsumedQuota: %d, ImageRatio: %f, AudioRatio: %f, AudioCompletionRatio: %f, ImageCompletionRatio: %f", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, p.UsePrice, p.CacheCreationRatio, p.ShouldPreConsumedQuota, p.ImageRatio, p.AudioRatio, p.AudioCompletionRatio, p.ImageCompletionRatio)
}
