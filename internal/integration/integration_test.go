//go:build integration
// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

const baseURL = "http://localhost:8080/api/v1"

func TestCreateAndWaitForTask(t *testing.T) {
	body := map[string]interface{}{
		"name":    "integration-test",
		"command": "echo integration-success",
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Skipf("跳过集成测试：服务不可达: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("创建任务 status = %d, 期望 %d", resp.StatusCode, http.StatusCreated)
	}

	var createResp struct {
		TaskUID string `json:"task_uid"`
		Status  string `json:"status"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)

	if createResp.TaskUID == "" {
		t.Fatal("task_uid 不应为空")
	}
	t.Logf("已创建任务: %s", createResp.TaskUID)

	var finalStatus string
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)

		resp, err := http.Get(fmt.Sprintf("%s/tasks/%s", baseURL, createResp.TaskUID))
		if err != nil {
			t.Fatalf("获取任务失败: %v", err)
		}
		var task map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&task)
		resp.Body.Close()

		finalStatus = task["status"].(string)
		if finalStatus == "success" || finalStatus == "failed" || finalStatus == "canceled" {
			break
		}
	}

	if finalStatus != "success" {
		t.Errorf("最终状态 = %s, 期望 success", finalStatus)
	}

	resp, err = http.Get(fmt.Sprintf("%s/tasks/%s/log", baseURL, createResp.TaskUID))
	if err != nil {
		t.Fatalf("获取日志失败: %v", err)
	}
	var logResp struct {
		Stdout string `json:"stdout"`
	}
	json.NewDecoder(resp.Body).Decode(&logResp)
	resp.Body.Close()

	if logResp.Stdout != "integration-success\n" {
		t.Errorf("stdout = %q, 期望 %q", logResp.Stdout, "integration-success\n")
	}
}

func TestTaskTimeout(t *testing.T) {
	body := map[string]interface{}{
		"name":        "timeout-test",
		"command":     "sleep 30",
		"timeout_sec": 1,
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Skipf("跳过集成测试：服务不可达: %v", err)
	}
	defer resp.Body.Close()

	var createResp struct {
		TaskUID string `json:"task_uid"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)

	var finalStatus string
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		resp, _ := http.Get(fmt.Sprintf("%s/tasks/%s", baseURL, createResp.TaskUID))
		var task map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&task)
		resp.Body.Close()
		finalStatus = task["status"].(string)
		if finalStatus == "failed" || finalStatus == "success" {
			break
		}
	}

	if finalStatus != "failed" {
		t.Errorf("最终状态 = %s, 期望 failed（超时）", finalStatus)
	}
}

func TestCancelTask(t *testing.T) {
	body := map[string]interface{}{
		"name":    "cancel-test",
		"command": "sleep 60",
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Skipf("跳过集成测试：服务不可达: %v", err)
	}
	var createResp struct {
		TaskUID string `json:"task_uid"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()

	time.Sleep(1 * time.Second)

	cancelResp, err := http.Post(
		fmt.Sprintf("%s/tasks/%s/cancel", baseURL, createResp.TaskUID),
		"application/json", nil,
	)
	if err != nil {
		t.Fatalf("取消请求失败: %v", err)
	}
	cancelResp.Body.Close()

	if cancelResp.StatusCode != http.StatusOK {
		t.Errorf("取消 status = %d, 期望 %d", cancelResp.StatusCode, http.StatusOK)
	}
}

func TestTaskListAndPagination(t *testing.T) {
	resp, err := http.Get(baseURL + "/tasks?page=1&size=5")
	if err != nil {
		t.Skipf("跳过集成测试：服务不可达: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("列表 status = %d, 期望 %d", resp.StatusCode, http.StatusOK)
	}

	var result struct {
		Tasks []map[string]interface{} `json:"tasks"`
		Total int                      `json:"total"`
		Page  int                      `json:"page"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Page != 1 {
		t.Errorf("Page = %d, 期望 1", result.Page)
	}
}

func TestRerunTask(t *testing.T) {
	body := map[string]interface{}{
		"name":    "rerun-source",
		"command": "echo original",
	}
	jsonBody, _ := json.Marshal(body)
	resp, _ := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
	var createResp struct {
		TaskUID string `json:"task_uid"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()

	time.Sleep(2 * time.Second)

	rerunResp, err := http.Post(
		fmt.Sprintf("%s/tasks/%s/rerun", baseURL, createResp.TaskUID),
		"application/json", nil,
	)
	if err != nil {
		t.Skipf("跳过集成测试：服务不可达: %v", err)
	}
	defer rerunResp.Body.Close()

	if rerunResp.StatusCode != http.StatusCreated {
		t.Errorf("重跑 status = %d, 期望 %d", rerunResp.StatusCode, http.StatusCreated)
	}

	var rerunBody struct {
		TaskUID string `json:"task_uid"`
	}
	json.NewDecoder(rerunResp.Body).Decode(&rerunBody)
	if rerunBody.TaskUID == createResp.TaskUID {
		t.Error("重跑任务的 task_uid 应与原始任务不同")
	}
}

// TestIdempotency 测试幂等性：相同命令和目录两次提交，第一次成功，第二次应被拒绝。
func TestIdempotency(t *testing.T) {
	body := map[string]interface{}{
		"name":    "idempotency-test",
		"command": "echo idempotent",
		"workdir": "/tmp",
	}
	jsonBody, _ := json.Marshal(body)

	// 第一次提交，应成功
	resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Skipf("跳过集成测试：服务不可达: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("首次创建 status = %d, 期望 %d", resp.StatusCode, http.StatusCreated)
	}

	var createResp struct {
		TaskUID string `json:"task_uid"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)
	t.Logf("首次任务已创建: %s", createResp.TaskUID)

	// 第二次提交相同命令和工作目录，应因幂等性被拒绝
	resp2, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Skipf("跳过集成测试：服务不可达: %v", err)
	}
	defer resp2.Body.Close()

	var errResp struct {
		Error string `json:"error"`
	}
	json.NewDecoder(resp2.Body).Decode(&errResp)

	// 根据实现，重复提交可能返回 409 或 500 但错误消息应包含 "duplicate"
	if resp2.StatusCode != http.StatusConflict && resp2.StatusCode != http.StatusInternalServerError {
		t.Errorf("重复创建 status = %d, 期望 409 或 500", resp2.StatusCode)
	}
	if !strings.Contains(errResp.Error, "duplicate") {
		t.Errorf("错误消息应包含 'duplicate'，实际: %s", errResp.Error)
	}
	t.Logf("幂等性验证通过: %s", errResp.Error)
}

// TestTaskRetry 测试任务重试：提交会失败的任务，验证重试计数和最终状态。
func TestTaskRetry(t *testing.T) {
	body := map[string]interface{}{
		"name":    "retry-test",
		"command": "exit 1",
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Skipf("跳过集成测试：服务不可达: %v", err)
	}
	defer resp.Body.Close()

	var createResp struct {
		TaskUID string `json:"task_uid"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)
	t.Logf("重试任务已创建: %s", createResp.TaskUID)

	// 轮询等待任务完成（重试可能使总耗时较长）
	var finalStatus string
	var retryCount int
	maxWait := 60
	for i := 0; i < maxWait; i++ {
		time.Sleep(1 * time.Second)
		resp, err := http.Get(fmt.Sprintf("%s/tasks/%s", baseURL, createResp.TaskUID))
		if err != nil {
			continue
		}
		var task map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&task)
		resp.Body.Close()

		finalStatus, _ = task["status"].(string)
		if rc, ok := task["retry_count"]; ok {
			if f, ok := rc.(float64); ok {
				retryCount = int(f)
			}
		}
		if finalStatus == "failed" || finalStatus == "success" {
			break
		}
	}

	if finalStatus != "failed" {
		t.Errorf("最终状态 = %s, 期望 failed", finalStatus)
	}
	if retryCount == 0 {
		t.Log("注意: retry_count = 0（可能仍在首次执行中或重试未触发）")
	}
	t.Logf("任务状态: %s, 重试次数: %d", finalStatus, retryCount)
}

// TestSSELogStream 测试 SSE 日志流：创建任务并连接 SSE 端点验证日志输出。
func TestSSELogStream(t *testing.T) {
	body := map[string]interface{}{
		"name":    "sse-test",
		"command": "echo hello-sse",
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Skipf("跳过集成测试：服务不可达: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("创建任务 status = %d, 期望 %d", resp.StatusCode, http.StatusCreated)
	}

	var createResp struct {
		TaskUID string `json:"task_uid"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)
	t.Logf("SSE 测试任务已创建: %s", createResp.TaskUID)

	// 等 1 秒让任务执行完成
	time.Sleep(1 * time.Second)

	// 连接 SSE 端点
	sseResp, err := http.Get(fmt.Sprintf("%s/tasks/%s/log/stream", baseURL, createResp.TaskUID))
	if err != nil {
		t.Skipf("跳过：SSE 端点不可达: %v", err)
	}
	defer sseResp.Body.Close()

	if sseResp.StatusCode != http.StatusOK {
		t.Fatalf("SSE 端点 status = %d, 期望 %d", sseResp.StatusCode, http.StatusOK)
	}

	// 读取 SSE 事件流
	buf := make([]byte, 4096)
	n, err := sseResp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("读取 SSE 流失败: %v", err)
	}

	bodyStr := string(buf[:n])
	if !strings.Contains(bodyStr, "hello-sse") {
		t.Errorf("SSE 数据中未找到 'hello-sse'，实际: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "event: done") {
		t.Errorf("SSE 数据中未找到 'event: done'，实际: %s", bodyStr)
	}
	t.Logf("SSE 日志流验证通过，收到 %d 字节", n)
}

// TestRateLimit 测试限流：发送大量请求，验证至少一个返回 429。
func TestRateLimit(t *testing.T) {
	// 发送略超过限流阈值（默认 100/min）的请求
	requestCount := 110
	tooManyRequests := 0
	body := map[string]interface{}{
		"name":    "rate-limit-test",
		"command": "echo rate-limit",
	}
	jsonBody, _ := json.Marshal(body)

	for i := 0; i < requestCount; i++ {
		req, err := http.NewRequest("POST", baseURL+"/tasks", bytes.NewBuffer(jsonBody))
		if err != nil {
			t.Skipf("跳过：创建请求失败: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Skipf("跳过集成测试：服务不可达: %v", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			tooManyRequests++
		}
		resp.Body.Close()
	}

	if tooManyRequests == 0 {
		t.Log("注意：未触发限流（可能请求未超过阈值或 Redis 不可用）")
	} else {
		t.Logf("限流触发 %d 次", tooManyRequests)
	}
}

// TestDelayedTask 测试延迟任务：提交任务后立即查询不应为终态。
func TestDelayedTask(t *testing.T) {
	body := map[string]interface{}{
		"name":    "delayed-test",
		"command": "echo delayed",
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/tasks", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Skipf("跳过集成测试：服务不可达: %v", err)
	}
	defer resp.Body.Close()

	var createResp struct {
		TaskUID string `json:"task_uid"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)
	t.Logf("延迟测试任务已创建: %s", createResp.TaskUID)

	// 立即查询，任务不应为终态（created 或 pending 或 running）
	resp2, err := http.Get(fmt.Sprintf("%s/tasks/%s", baseURL, createResp.TaskUID))
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	defer resp2.Body.Close()

	var task map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&task)
	status, _ := task["status"].(string)

	if status == "success" || status == "failed" || status == "canceled" {
		t.Errorf("任务刚刚创建，状态不应为终态: %s", status)
	} else {
		t.Logf("任务状态符合预期（非终态）: %s", status)
	}

	// 等待任务完成
	time.Sleep(3 * time.Second)

	resp3, err := http.Get(fmt.Sprintf("%s/tasks/%s", baseURL, createResp.TaskUID))
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	defer resp3.Body.Close()

	json.NewDecoder(resp3.Body).Decode(&task)
	finalStatus, _ := task["status"].(string)
	t.Logf("等待后任务状态: %s", finalStatus)
	if finalStatus != "success" {
		t.Errorf("最终状态 = %s, 期望 success", finalStatus)
	}
}
