package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateAlphaNumDash(t *testing.T) {
	type testStruct struct {
		Name string `validate:"alphanumdash"`
	}

	tests := []struct {
		input string
		want  bool
	}{
		{"abc", true},
		{"ABC", true},
		{"abc123", true},
		{"test_label", true},
		{"test-label", true},
		{"Test_Label-123", true},
		{"", false},
		{"test label", false},
		{"test@label", false},
		{"test.label", false},
		{"test/label", false},
		{"test!", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := validate.Struct(testStruct{Name: tt.input})
			got := err == nil
			if got != tt.want {
				t.Errorf("alphanumdash(%q) valid = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHandleBindError_Required(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Name string `json:"name" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBindError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	details, ok := resp["details"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected details, got: %v", resp)
	}
	msg, _ := details["Name"].(string)
	if !strings.Contains(msg, "required") {
		t.Errorf("expected 'required' in message, got %q", msg)
	}
}

func TestHandleBindError_Min(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Count int `json:"count" binding:"min=5"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBindError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"count":1}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	details := resp["details"].(map[string]interface{})
	msg := details["Count"].(string)
	if !strings.Contains(msg, "at least 5") {
		t.Errorf("expected 'at least 5', got %q", msg)
	}
}

func TestHandleBindError_Max(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Count int `json:"count" binding:"max=10"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBindError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"count":20}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	details := resp["details"].(map[string]interface{})
	msg := details["Count"].(string)
	if !strings.Contains(msg, "at most 10") {
		t.Errorf("expected 'at most 10', got %q", msg)
	}
}

func TestHandleBindError_Oneof(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Type string `json:"type" binding:"oneof=direct socks5 socks4"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBindError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"type":"invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	details := resp["details"].(map[string]interface{})
	msg := details["Type"].(string)
	if !strings.Contains(msg, "one of") {
		t.Errorf("expected 'one of', got %q", msg)
	}
}

func TestHandleBindError_Len(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Code string `json:"code" binding:"len=6"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBindError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"code":"abc"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	details := resp["details"].(map[string]interface{})
	msg := details["Code"].(string)
	if !strings.Contains(msg, "exactly 6 characters") {
		t.Errorf("expected 'exactly 6 characters', got %q", msg)
	}
}

func TestHandleBindError_AlphaNumDashMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Test alphanumdash validation message through the package-level validate variable
	type testStruct struct {
		Name string `validate:"alphanumdash"`
	}
	err := validate.Struct(testStruct{Name: "bad name!"})
	if err == nil {
		t.Fatal("expected validation error for 'bad name!'")
	}
	// The HandleBindError function handles alphanumdash tag specifically
	// We can't test through Gin binding since alphanumdash is registered on
	// the local validate instance, not Gin's binding validator.
}

func TestHandleBindError_Alpha(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Code string `json:"code" binding:"alpha"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBindError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"code":"abc123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	details := resp["details"].(map[string]interface{})
	msg := details["Code"].(string)
	if !strings.Contains(msg, "only letters") {
		t.Errorf("expected 'only letters', got %q", msg)
	}
}

func TestHandleBindError_Numeric(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Num string `json:"num" binding:"numeric"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBindError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"num":"abc"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	details := resp["details"].(map[string]interface{})
	msg := details["Num"].(string)
	if !strings.Contains(msg, "numeric") {
		t.Errorf("expected 'numeric', got %q", msg)
	}
}

func TestHandleBindError_Hexadecimal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Hex string `json:"hex" binding:"hexadecimal"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBindError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"hex":"xyz"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	details := resp["details"].(map[string]interface{})
	msg := details["Hex"].(string)
	if !strings.Contains(msg, "hex") {
		t.Errorf("expected 'hex' message, got %q", msg)
	}
}

func TestHandleBindError_UnknownTag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Val string `json:"val" binding:"url"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBindError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"val":"not a url!!!"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	details := resp["details"].(map[string]interface{})
	msg := details["Val"].(string)
	if !strings.Contains(msg, "failed validation") {
		t.Errorf("expected 'failed validation' for unknown tag, got %q", msg)
	}
}

func TestHandleBindError_NonValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/test", func(c *gin.Context) {
		var req struct {
			Name string `json:"name" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBindError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, hasDetails := resp["details"]; hasDetails {
		t.Error("non-validation error should not have 'details' key")
	}
	if _, hasError := resp["error"]; !hasError {
		t.Error("expected 'error' key in response")
	}
}
