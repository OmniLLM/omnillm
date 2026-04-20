package server

import (
	"html"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func registerAdminUIRoutes(r *gin.Engine, apiKey string) {
	assetsDir := filepath.Clean(filepath.Join("pages", "admin", "assets"))
	if _, err := os.Stat(assetsDir); err == nil {
		r.Static("/admin/assets", assetsDir)
	}

	r.GET("/admin/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderAdminIndex(apiKey)))
	})
}

func renderAdminIndex(apiKey string) string {
	indexPath := filepath.Clean(filepath.Join("pages", "admin", "index.html"))
	content, err := os.ReadFile(indexPath)
	if err != nil {
		fallback := "<!doctype html><html><head><meta charset=\"UTF-8\" /><meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\" /><title>OmniLLM Admin</title></head><body><div id=\"root\"></div></body></html>"
		return injectAPIKeyMeta(fallback, apiKey)
	}
	return injectAPIKeyMeta(string(content), apiKey)
}

func injectAPIKeyMeta(pageHTML string, apiKey string) string {
	metaTag := fmt.Sprintf(`<meta name="omnillm-api-key" content="%s" />`, html.EscapeString(apiKey))
	if strings.Contains(pageHTML, `<meta name="omnillm-api-key"`) {
		return strings.Replace(
			pageHTML,
			`<meta name="omnillm-api-key" content="" />`,
			metaTag,
			1,
		)
	}
	if strings.Contains(pageHTML, "<title>") {
		return strings.Replace(pageHTML, "<title>", metaTag+"\n    <title>", 1)
	}
	return metaTag + pageHTML
}
