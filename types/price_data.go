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
	return fmt.Sprintf("ModelPrice: %g, ModelRatio: %g, CompletionRatio: %g, CacheRatio: %g, GroupRatio: %g, UsePrice: %t, CacheCreationRatio: %g, ShouldPreConsumedQuota: %d, ImageRatio: %g", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, p.UsePrice, p.CacheCreationRatio, p.ShouldPreConsumedQuota, p.ImageRatio)
}
