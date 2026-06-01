//go:build integration
// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
