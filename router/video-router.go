package router

import (
	"one-api/controller"
	"one-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	videoV1Router := router.Group("/v1")
	// 视频内容下载端点（不需要认证）
	videoV1Router.GET("/videos/:task_id/content", controller.VideoProxy)
	videoV1Router.GET("/video/generations/:task_id/content/:gen_id", controller.VideoProxy) // Azure Sora 格式

	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.POST("/video/generations", controller.RelayTask)
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTask)
	}
	// openai compatible API video routes
	// docs: https://platform.openai.com/docs/api-reference/videos/create
	{
		videoV1Router.POST("/videos", controller.RelayTask)
		videoV1Router.GET("/videos/:task_id", controller.RelayTask)
	}

	klingV1Router := router.Group("/kling/v1")
	klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		klingV1Router.POST("/videos/text2video", controller.RelayTask)
		klingV1Router.POST("/videos/image2video", controller.RelayTask)
		klingV1Router.GET("/videos/text2video/:task_id", controller.RelayTask)
		klingV1Router.GET("/videos/image2video/:task_id", controller.RelayTask)
	}

	// Jimeng official API routes - direct mapping to official API format
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", controller.RelayTask)
	}

	// Doubao video generation API routes
	doubaoV1Router := router.Group("/doubao/v1")
	doubaoV1Router.Use(middleware.DoubaoRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		doubaoV1Router.POST("/videos/text2video", controller.RelayTask)
		doubaoV1Router.POST("/videos/image2video", controller.RelayTask)
		doubaoV1Router.GET("/videos/text2video/:task_id", controller.RelayTask)
		doubaoV1Router.GET("/videos/image2video/:task_id", controller.RelayTask)
	}
}
