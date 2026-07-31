package middleware

import (
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func NewCorsMiddleware() *CorsMiddleware {
	return &CorsMiddleware{}
}

type CorsMiddleware struct{}

func isAllowedOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}

	return host == "muxixyz.com" || strings.HasSuffix(host, ".muxixyz.com")
}

func (c *CorsMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return cors.New(cors.Config{
		// 允许的请求头
		AllowHeaders: []string{"Content-Type", "Authorization"},
		// 添加到响应头去,默认的响应头是不能够显示自定义的部分的
		ExposeHeaders: []string{"x-jwt-token", "x-refresh-token", "X-Trace-Id"}, //前面那两个不太敢动,正常格式是要大小写对齐的,等之后有了测试环境再试
		// 是否允许携带凭证（如 Cookies）
		AllowCredentials: true,
		// 仅允许本地开发环境和 muxixyz.com 的可信子域。
		AllowOriginFunc: isAllowedOrigin,

		// 预检请求的缓存时间
		MaxAge: 12 * time.Hour,
	})
}
