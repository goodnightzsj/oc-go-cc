package gui

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"syscall"

	"github.com/routatic/proxy/internal/config"
)

// Messages are fixed, safe UI copy. Never send a raw error, path, submitted
// value or reverse-proxy response to the settings page as diagnostic detail.
var settingsErrorMessages = map[string]string{
	"invalid_config":          "配置未通过校验，请检查设置项。",
	"invalid_json":            "配置格式不正确，请检查 JSON 的逗号、引号和括号。",
	"invalid_type":            "设置值的类型不正确，请检查数字、开关或列表格式。",
	"object_required":         "配置必须是 JSON 对象，不能是列表或空值。",
	"missing_api_key":         "请至少为一个平台填写 API 密钥。",
	"unresolved_env":          "配置引用的环境变量未设置，请在服务器设置变量或直接填写该项。",
	"invalid_environment":     "服务器环境变量格式不正确，请检查 AWS 账单开关是否为 true 或 false。",
	"empty_key":               "密钥列表中有空项，请补全或删除。",
	"masked_keys_mixed":       "不能把隐藏的旧密钥和新密钥混填，请重新填写完整的密钥列表。",
	"port_range":              "端口请输入 1–65535 的整数；填 0 使用默认端口。",
	"absolute_url":            "请填写完整的 http:// 或 https:// 地址。",
	"plain_url":               "请填写完整的 HTTP 或 HTTPS 地址，不要包含用户名、密码或 # 片段。",
	"non_negative":            "超时时间不能为负数，请填写 0 或正整数，单位为毫秒。",
	"channel_required":        "请填写目标渠道，例如 deepseek。",
	"channel_slug":            "渠道名请使用小写字母或数字，词之间用单个短横线连接，例如 deepseek、z-ai。",
	"model_required":          "请填写模型 ID。",
	"unknown_provider":        "平台名称不受支持，请从已有平台中选择。",
	"wire_format":             "模型协议不正确，请选择 auto、openai、anthropic、responses 或 gemini。",
	"commandcode_wire_format": "CommandCode 只支持 OpenAI 或 Anthropic 协议，请修改模型协议。",
	"cline_wire_format":       "ClinePass 只支持 OpenAI 协议，请修改模型协议。",
	"empty_family":            "模型系列名称不能为空，请填写后再保存。",
	"vision_required":         "视觉场景需要支持图片的模型，请更换模型或调整视觉设置。",
	"unknown_scenario":        "路由场景名称不受支持，请检查场景配置。",
	"account_id":              "AWS 账单需要明确的账号范围，请填写 12 位数字账号 ID。",
	"stored_config_invalid":   "服务器现有配置文件已损坏，请先修复配置文件再保存。",
	"file_permission":         "服务器无法读写配置文件，请检查文件和目录权限。",
	"file_missing":            "服务器找不到配置文件，请检查配置路径。",
	"disk_full":               "服务器磁盘空间不足，请清理空间后重试。",
	"config_io":               "服务器无法读写配置，请检查文件路径、权限和磁盘状态。",
	"config_unavailable":      "配置尚未加载，请稍后重新加载页面。",
	"autostart_failed":        "系统未能保存开机自启动设置，请检查当前用户的系统权限。",
	"redaction_failed":        "配置无法安全展示，请重新加载；不要重复提交已保存的配置。",
	"export_failed":           "配置导出失败，请稍后重试。",
}

type settingsError struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Saved   bool   `json:"saved"`
}

func writeSettingsError(w http.ResponseWriter, status int, code, field string, saved bool) {
	message, ok := settingsErrorMessages[code]
	if !ok {
		code, message = "invalid_config", settingsErrorMessages["invalid_config"]
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]settingsError{"error": {Code: code, Field: field, Message: message, Saved: saved}})
}

func writeConfigError(w http.ResponseWriter, err error) {
	status, code, field := http.StatusBadRequest, "invalid_config", ""
	var validation *config.ValidationError
	var pathErr *os.PathError
	var linkErr *os.LinkError
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &validation):
		code, field = validation.Code, validation.Field
	case errors.As(err, &typeErr):
		code, field = "invalid_type", typeErr.Field
	case errors.As(err, &syntaxErr):
		code = "invalid_json"
	case errors.As(err, &pathErr), errors.As(err, &linkErr):
		status, code = http.StatusInternalServerError, "config_io"
		if errors.Is(err, os.ErrPermission) {
			code = "file_permission"
		} else if errors.Is(err, os.ErrNotExist) {
			code = "file_missing"
		} else if errors.Is(err, syscall.ENOSPC) {
			code = "disk_full"
		}
	}
	writeSettingsError(w, status, code, field, false)
}
