package plugin

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func configureSettingsApp(t *testing.T, statePath string) *App {
	t.Helper()
	app := NewApp()
	configYAML := []byte("enabled: true\nstate_file: \"" + filepath.ToSlash(statePath) + "\"\nglobal_weighted_round_robin: false\n")
	request, err := json.Marshal(LifecycleRequest{ConfigYAML: configYAML})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.HandleMethod(MethodPluginReconfigure, request); err != nil {
		t.Fatalf("配置调度设置测试应用失败: %v", err)
	}
	return app
}

func callManagementForTest(t *testing.T, app *App, method, path string, body []byte) ManagementResponse {
	t.Helper()
	request, err := json.Marshal(ManagementRequest{Method: method, Path: path, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := app.HandleMethod(MethodManagementHandle, request)
	if err != nil {
		t.Fatal(err)
	}
	return managementResponseFromEnvelope(t, raw)
}

func assertGlobalWeightedSetting(t *testing.T, response ManagementResponse, expectedStatus int, expectedValue bool) {
	t.Helper()
	if response.StatusCode != expectedStatus {
		t.Fatalf("设置接口状态码 = %d，期望 %d，响应体 = %s", response.StatusCode, expectedStatus, response.Body)
	}
	var payload struct {
		GlobalWeightedRoundRobin bool `json:"global_weighted_round_robin"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		t.Fatalf("解析设置接口响应失败: %v，响应体 = %s", err, response.Body)
	}
	if payload.GlobalWeightedRoundRobin != expectedValue {
		t.Fatalf("全局加权轮询 = %v，期望 %v", payload.GlobalWeightedRoundRobin, expectedValue)
	}
}
