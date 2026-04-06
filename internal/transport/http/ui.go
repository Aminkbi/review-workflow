package httptransport

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed ui/index.html ui/assets/*
var uiFiles embed.FS

func registerUI(router *gin.Engine) {
	assets, err := fs.Sub(uiFiles, "ui/assets")
	if err != nil {
		panic(err)
	}
	indexHTML, err := uiFiles.ReadFile("ui/index.html")
	if err != nil {
		panic(err)
	}

	router.GET("/", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Data(200, "text/html; charset=utf-8", indexHTML)
	})
	router.StaticFS("/assets", http.FS(assets))
}
