package web

import (
	"embed"
	"io/fs"
)

// embedded 打包模板与静态资源，使 gonav 可编译为单文件部署。
//
//go:embed templates/*.html assets
var embedded embed.FS

// assetsFS 返回静态资源子文件系统。
func assetsFS() (fs.FS, error) {
	return fs.Sub(embedded, "assets")
}
