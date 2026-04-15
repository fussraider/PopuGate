package logger

import (
	"time"

	"github.com/gin-gonic/gin"
)

// GinLogger returns a gin.HandlerFunc that logs HTTP requests using the unified logger.
func GinLogger() gin.HandlerFunc {
	log := WithScope("http")
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		elapsed := time.Since(start)
		status := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path

		msg := "%d %s %s %v %s"
		args := []any{status, method, path, elapsed, c.ClientIP()}

		if status >= 500 {
			log.Errorf(msg, args...)
		} else if status >= 400 {
			log.Warnf(msg, args...)
		} else {
			log.Infof(msg, args...)
		}
	}
}
