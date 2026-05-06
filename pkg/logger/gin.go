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
			msg += " [Errors: %v]"
			args = append(args, c.Errors.String())
			log.Errorf(msg, args...)
		} else if status >= 400 {
			if len(c.Errors) > 0 {
				msg += " [Errors: %v]"
				args = append(args, c.Errors.String())
			}
			log.Warnf(msg, args...)
		} else {
			if len(c.Errors) > 0 {
				msg += " [Errors: %v]"
				args = append(args, c.Errors.String())
			}
			log.Infof(msg, args...)
		}
	}
}
