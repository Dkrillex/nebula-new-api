package types

import "fmt"

type GroupRatioInfo struct {
	GroupRatio        float64
	GroupSpecialRatio float64
	HasSpecialRatio   bool
}

type PriceData struct {
	ModelPrice             float64
	ModelRatio             float64
	CompletionRatio        float64
	CacheRatio             float64
	CacheCreationRatio     float64
	ImageRatio             float64
	UsePrice               bool
	ShouldPreConsumedQuota int
	GroupRatioInfo         GroupRatioInfo
}

type PerCallPriceData struct {
	ModelPrice     float64
	Quota          int
	GroupRatioInfo GroupRatioInfo
}

func (p PriceData) ToSetting() string {
	return fmt.Sprintf("ModelPrice: %.6f, ModelRatio: %.6f, CompletionRatio: %.6f, CacheRatio: %.6f, GroupRatio: %.6f, UsePrice: %t, CacheCreationRatio: %.6f, ShouldPreConsumedQuota: %d, ImageRatio: %.6f", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, p.UsePrice, p.CacheCreationRatio, p.ShouldPreConsumedQuota, p.ImageRatio)
}
