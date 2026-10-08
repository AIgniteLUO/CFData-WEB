package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// 环境变量统一使用 CFDATA_ 前缀，避免与容器内其他工具的环境变量撞名（裸 PORT、USER、HOST 都很常见）。
// 优先级：命令行参数 > 环境变量 > 默认值。
const envPrefix = "CFDATA_"

// 会话时长的上界来自 time.Duration 的表示范围：int64 纳秒除以每分钟纳秒数取整即 153722867。
// 超过它，main.go 里 time.Duration(webSessionMinutes) * time.Minute 会溢出成负数，
// 结果是 cookie 的 MaxAge 为负、会话立即失效，用户表现为「密码明明正确却一直跳回登录页」且日志无任何报错。
const maxSessionMinutes = 153722867

// CFDATA_DEBUG 只接受 setDebugFlag 能明确识别的值，其余一律告警忽略。
// 命令行 -debug 保持原有的宽松行为不变（无法识别的值会按 error 级开启调试），
// 但环境变量是运维手写的，一个笔误就静默打开调试日志并不划算。
var debugEnvValues = map[string]struct{}{
	"true": {}, "false": {}, "1": {}, "0": {},
	"yes": {}, "no": {}, "y": {}, "n": {}, "on": {}, "off": {},
	"error": {}, "err": {}, "program": {}, "all": {}, "full": {}, "全部": {},
}

// envString 读取环境变量；未设置或只有空白时都视为未设置，这样 compose 里 CFDATA_USER= 这类空占位不会产生意外值。
func envString(name string) (string, bool) {
	value, ok := os.LookupEnv(envPrefix + name)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

// envInt 读取整数环境变量，非法或越界时告警并忽略，让程序照常启动以免容器起不来。
func envInt(name string, min, max int) (int, bool) {
	value, ok := envString(name)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < min || parsed > max {
		warnInvalidEnv(name, value, fmt.Sprintf("需为 %d 到 %d 之间的整数", min, max))
		return 0, false
	}
	return parsed, true
}

// envBool 读取布尔环境变量。返回值第二个参数表示“环境变量是否被显式设置”，
// 因此 CFDATA_SKIPGEO=false 能正确覆盖默认值，而不是被当作未设置。
// 接受的写法与命令行 -skipgeo 一致（含 t/f）。
func envBool(name string) (bool, bool) {
	value, ok := envString(name)
	if !ok {
		return false, false
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "y", "t", "on":
		return true, true
	case "0", "false", "no", "n", "f", "off":
		return false, true
	}
	warnInvalidEnv(name, value, "需为 true/false、1/0、yes/no、t/f 或 on/off")
	return false, false
}

func warnInvalidEnv(name, value, expect string) {
	fmt.Printf("环境变量 %s%s=%q 无效（%s），已忽略\n", envPrefix, name, value, expect)
}

// applyEnvDefaults 必须在所有 flag 注册之后、flag.Parse() 之前调用。
//
// flag.IntVar/StringVar 注册时就把默认值写进了变量，而 Parse 只在参数真正出现时才覆盖，
// 所以在两者之间写入环境变量的值，天然形成「命令行参数 > 环境变量 > 默认值」的优先级。
// 特意不用 flag.Set()：它会把 flag 标记为“已显式设置”，破坏 flag.Visit 对 -dns 是否显式传入的判断。
func applyEnvDefaults() {
	if value, ok := envInt("PORT", 1, 65535); ok {
		listenPort = value
	}
	if value, ok := envString("HOST"); ok {
		listenHost = value
	}
	if value, ok := envString("USER"); ok {
		webUser = value
	}
	if value, ok := envString("PASSWORD"); ok {
		webPassword = value
	}
	if value, ok := envInt("SESSION", 1, maxSessionMinutes); ok {
		webSessionMinutes = value
	}
	if value, ok := envBool("SKIPGEO"); ok {
		skipGeoCheck = value
	}
	if value, ok := envString("DEBUG"); ok {
		if _, valid := debugEnvValues[strings.ToLower(value)]; !valid {
			warnInvalidEnv("DEBUG", value, "可选 error 或 all，或 true/false 开关")
		} else {
			_ = setDebugFlag(value)
		}
	}
}
