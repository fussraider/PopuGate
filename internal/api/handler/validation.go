package handler

import (
	"errors"
	"fmt"
	"net/http"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
	_ = validate.RegisterValidation("alphanumdash", validateAlphaNumDash)

	// Also register on Gin's binding validator so ShouldBindJSON recognizes it.
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		_ = v.RegisterValidation("alphanumdash", validateAlphaNumDash)
	}
}

func validateAlphaNumDash(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// HandleBindError processes ShouldBindJSON errors and returns a structured response.
func HandleBindError(c *gin.Context, err error) {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		details := make(map[string]string, len(ve))
		for _, fe := range ve {
			field := fe.Field()
			switch fe.Tag() {
			case "required":
				details[field] = fmt.Sprintf("%s is required", field)
			case "min":
				details[field] = fmt.Sprintf("%s must be at least %s", field, fe.Param())
			case "max":
				details[field] = fmt.Sprintf("%s must be at most %s", field, fe.Param())
			case "len":
				details[field] = fmt.Sprintf("%s must be exactly %s characters", field, fe.Param())
			case "oneof":
				details[field] = fmt.Sprintf("%s must be one of: %s", field, fe.Param())
			case "alphanumdash":
				details[field] = fmt.Sprintf("%s must contain only letters, digits, underscores, and hyphens", field)
			case "alpha":
				details[field] = fmt.Sprintf("%s must contain only letters", field)
			case "numeric":
				details[field] = fmt.Sprintf("%s must be numeric", field)
			case "hexadecimal":
				details[field] = fmt.Sprintf("%s must be a valid hex string", field)
			default:
				details[field] = fmt.Sprintf("%s failed validation: %s", field, fe.Tag())
			}
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation failed", "details": details})
		return
	}

	msg := err.Error()
	if msg == "" {
		msg = "invalid request body"
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": msg})
}
