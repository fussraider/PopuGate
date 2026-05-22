package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/scheduler"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func setupSchedulerTestEnv(t *testing.T) (*gin.Engine, *SchedulerHandler, *scheduler.Scheduler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	schedulerStore := store.NewSchedulerStore(db)

	sched := scheduler.New()
	tasks := []scheduler.Task{
		{Name: "traffic-flush", Schedule: "0 */1 * * * *", Fn: func(_ context.Context) error { return nil }},
		{Name: "quota-check", Schedule: "0 */5 * * * *", Fn: func(_ context.Context) error { return nil }},
	}
	sched.StartWith(tasks, nil, schedulerStore)
	t.Cleanup(func() { sched.Stop() })

	svc := service.NewSchedulerService(schedulerStore, sched)
	handler := NewSchedulerHandler(svc)

	r := gin.New()
	r.GET("/api/v1/scheduler/tasks", handler.List)
	r.PUT("/api/v1/scheduler/tasks/:name", handler.Update)
	r.POST("/api/v1/scheduler/tasks/:name/run", handler.RunNow)
	r.GET("/api/v1/scheduler/tasks/:name/history", handler.History)
	r.GET("/api/v1/scheduler/history", handler.AllHistory)

	return r, handler, sched
}

func TestSchedulerHandler_List(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scheduler/tasks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tasks []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(tasks) == 0 {
		t.Error("expected at least one task")
	}

	// Verify task structure
	for _, task := range tasks {
		if _, ok := task["name"]; !ok {
			t.Error("task missing 'name' field")
		}
		if _, ok := task["enabled"]; !ok {
			t.Error("task missing 'enabled' field")
		}
		if _, ok := task["default_schedule"]; !ok {
			t.Error("task missing 'default_schedule' field")
		}
	}
}

func TestSchedulerHandler_Update_ValidEnable(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	enabled := false
	body, _ := json.Marshal(map[string]interface{}{
		"enabled": enabled,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/scheduler/tasks/traffic-flush", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSchedulerHandler_Update_ValidSchedule(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	body, _ := json.Marshal(map[string]interface{}{
		"schedule": "0 */2 * * * *",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/scheduler/tasks/traffic-flush", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSchedulerHandler_Update_InvalidJSON(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/scheduler/tasks/traffic-flush", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSchedulerHandler_Update_NothingToUpdate(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/scheduler/tasks/traffic-flush", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "nothing to update" {
		t.Errorf("expected 'nothing to update', got %v", resp["error"])
	}
}

func TestSchedulerHandler_Update_UnknownTask(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	enabled := true
	body, _ := json.Marshal(map[string]interface{}{
		"enabled": enabled,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/scheduler/tasks/nonexistent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSchedulerHandler_Update_InvalidCron(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	body, _ := json.Marshal(map[string]interface{}{
		"schedule": "not-a-cron",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/scheduler/tasks/traffic-flush", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSchedulerHandler_RunNow(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/tasks/traffic-flush/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["task_name"] != "traffic-flush" {
		t.Errorf("expected task_name 'traffic-flush', got %v", resp["task_name"])
	}
	if resp["status"] != "success" {
		t.Errorf("expected status 'success', got %v", resp["status"])
	}
}

func TestSchedulerHandler_RunNow_UnknownTask(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/tasks/nonexistent/run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSchedulerHandler_History(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scheduler/tasks/traffic-flush/history", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var records []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &records)
	// May be empty, but should be a valid array
	if records == nil {
		t.Error("expected non-nil array")
	}
}

func TestSchedulerHandler_AllHistory(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scheduler/history", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var records []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &records)
	if records == nil {
		t.Error("expected non-nil array")
	}
}

func TestSchedulerHandler_History_WithPagination(t *testing.T) {
	r, _, _ := setupSchedulerTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scheduler/tasks/traffic-flush/history?limit=5&offset=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		limitStr   string
		offsetStr  string
		wantLimit  int
		wantOffset int
	}{
		{"defaults", "", "", 20, 0},
		{"custom limit", "50", "", 50, 0},
		{"custom offset", "", "10", 20, 10},
		{"both custom", "30", "5", 30, 5},
		{"limit too high", "200", "", 20, 0},
		{"limit zero", "0", "", 20, 0},
		{"limit negative", "-5", "", 20, 0},
		{"offset negative", "-1", "", 20, 0},
		{"invalid limit", "abc", "", 20, 0},
		{"invalid offset", "", "xyz", 20, 0},
		{"limit at max", "100", "", 100, 0},
		{"limit over max", "101", "", 20, 0},
		{"limit 1", "1", "", 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/test", func(c *gin.Context) {
				limit, offset := getPagination(c)
				if limit != tt.wantLimit {
					t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
				}
				if offset != tt.wantOffset {
					t.Errorf("offset = %d, want %d", offset, tt.wantOffset)
				}
			})

			url := "/test"
			params := []string{}
			if tt.limitStr != "" {
				params = append(params, "limit="+tt.limitStr)
			}
			if tt.offsetStr != "" {
				params = append(params, "offset="+tt.offsetStr)
			}
			if len(params) > 0 {
				url += "?" + params[0]
				for _, p := range params[1:] {
					url += "&" + p
				}
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		})
	}
}
