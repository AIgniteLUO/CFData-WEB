package main

import (
	"flag"
	"os"
	"testing"
	"time"
)

// 环境变量涉及的配置项都是包级全局变量，测试之间必须还原，避免互相污染。
func snapshotGlobalConfig() func() {
	port, host, session := listenPort, listenHost, webSessionMinutes
	user, password := webUser, webPassword
	url, dns := speedTestURL, customDNSServer
	skipGeo, dbgMode, dbgLevel := skipGeoCheck, debugMode, debugLevel
	return func() {
		listenPort, listenHost, webSessionMinutes = port, host, session
		webUser, webPassword = user, password
		speedTestURL, customDNSServer = url, dns
		skipGeoCheck, debugMode, debugLevel = skipGeo, dbgMode, dbgLevel
	}
}

// resetGlobalConfigToDefaults 为「只测 applyEnvDefaults」的单元测试准备一份已知现场。
// 注意默认值的权威来源是 registerServerFlags()，由 TestServerFlagContract 守护；
// 这里的赋值只用于隔离被测函数，不承担校验默认值的职责。
func resetGlobalConfigToDefaults() {
	listenPort = 13335
	listenHost = ""
	webUser = ""
	webPassword = ""
	webSessionMinutes = 720
	skipGeoCheck = false
	debugMode = false
	debugLevel = "error"
}

func TestApplyEnvDefaultsUnsetKeepsDefaults(t *testing.T) {
	restore := snapshotGlobalConfig()
	defer restore()
	resetGlobalConfigToDefaults()

	applyEnvDefaults()

	if listenPort != 13335 {
		t.Errorf("未设置环境变量时 listenPort 应保持 13335，实际 %d", listenPort)
	}
	if listenHost != "" {
		t.Errorf("未设置环境变量时 listenHost 应保持空，实际 %q", listenHost)
	}
	if webUser != "" || webPassword != "" {
		t.Errorf("未设置环境变量时认证应保持关闭，实际 user=%q password=%q", webUser, webPassword)
	}
	if webSessionMinutes != 720 {
		t.Errorf("未设置环境变量时会话有效期应保持 720，实际 %d", webSessionMinutes)
	}
	if skipGeoCheck {
		t.Error("未设置环境变量时 skipGeoCheck 应保持 false")
	}
	if debugMode || debugLevel != "error" {
		t.Errorf("未设置环境变量时应保持非调试状态，实际 mode=%v level=%q", debugMode, debugLevel)
	}
	if speedTestURL != "" || customDNSServer != "" {
		t.Errorf("本次未提供环境变量的项不应被触碰，实际 url=%q dns=%q", speedTestURL, customDNSServer)
	}
}

func TestApplyEnvDefaultsReadsValues(t *testing.T) {
	restore := snapshotGlobalConfig()
	defer restore()
	resetGlobalConfigToDefaults()

	t.Setenv("CFDATA_PORT", "8080")
	t.Setenv("CFDATA_HOST", "0.0.0.0")
	t.Setenv("CFDATA_USER", "admin")
	t.Setenv("CFDATA_PASSWORD", "s3cret")
	t.Setenv("CFDATA_SESSION", "60")
	t.Setenv("CFDATA_SKIPGEO", "true")
	t.Setenv("CFDATA_DEBUG", "all")

	applyEnvDefaults()

	if listenPort != 8080 {
		t.Errorf("listenPort 应为 8080，实际 %d", listenPort)
	}
	if listenHost != "0.0.0.0" {
		t.Errorf("listenHost 应为 0.0.0.0，实际 %q", listenHost)
	}
	if webUser != "admin" {
		t.Errorf("webUser 应为 admin，实际 %q", webUser)
	}
	if webPassword != "s3cret" {
		t.Errorf("webPassword 应为 s3cret，实际 %q", webPassword)
	}
	if webSessionMinutes != 60 {
		t.Errorf("webSessionMinutes 应为 60，实际 %d", webSessionMinutes)
	}
	if !skipGeoCheck {
		t.Error("skipGeoCheck 应为 true")
	}
	if !debugMode || debugLevel != "all" {
		t.Errorf("调试等级应为 all 且开启，实际 mode=%v level=%q", debugMode, debugLevel)
	}
}

// 布尔值的显式 false 必须被采纳，不能被当成“未设置”而回落到默认值。
func TestApplyEnvDefaultsHonorsExplicitFalse(t *testing.T) {
	restore := snapshotGlobalConfig()
	defer restore()
	resetGlobalConfigToDefaults()

	skipGeoCheck = true
	debugMode = true
	debugLevel = "all"
	t.Setenv("CFDATA_SKIPGEO", "false")
	t.Setenv("CFDATA_DEBUG", "false")

	applyEnvDefaults()

	if skipGeoCheck {
		t.Error("CFDATA_SKIPGEO=false 应关闭 skipGeoCheck")
	}
	if debugMode {
		t.Error("CFDATA_DEBUG=false 应关闭调试模式")
	}
}

// 接受的布尔写法应与命令行 -skipgeo 保持一致，含 t/f 简写。
func TestApplyEnvDefaultsAcceptsShorthandBooleans(t *testing.T) {
	for value, want := range map[string]bool{"t": true, "f": false, "T": true, "F": false, "on": true, "off": false} {
		t.Run(value, func(t *testing.T) {
			restore := snapshotGlobalConfig()
			defer restore()
			resetGlobalConfigToDefaults()
			skipGeoCheck = !want
			t.Setenv("CFDATA_SKIPGEO", value)

			applyEnvDefaults()

			if skipGeoCheck != want {
				t.Errorf("CFDATA_SKIPGEO=%q 应解析为 %v，实际 %v", value, want, skipGeoCheck)
			}
		})
	}
}

// 非法值只告警并忽略，不能让程序起不来，也不能污染成 0 之类的危险值。
// 各项校验范围不同（端口 1-65535、会话 1-153722867、布尔是另一套字面量），必须分开造非法值。
func TestApplyEnvDefaultsIgnoresInvalidValues(t *testing.T) {
	t.Run("非法端口回落到默认值", func(t *testing.T) {
		for _, value := range []string{"not-a-port", "70000", "-1", "0", ""} {
			t.Run(value, func(t *testing.T) {
				restore := snapshotGlobalConfig()
				defer restore()
				resetGlobalConfigToDefaults()
				t.Setenv("CFDATA_PORT", value)

				applyEnvDefaults()

				if listenPort != 13335 {
					t.Errorf("非法值 %q 不应改变 listenPort，实际 %d", value, listenPort)
				}
			})
		}
	})

	t.Run("非法会话时长回落到默认值", func(t *testing.T) {
		for _, value := range []string{"forever", "153722868", "-1", "0", ""} {
			t.Run(value, func(t *testing.T) {
				restore := snapshotGlobalConfig()
				defer restore()
				resetGlobalConfigToDefaults()
				t.Setenv("CFDATA_SESSION", value)

				applyEnvDefaults()

				if webSessionMinutes != 720 {
					t.Errorf("非法值 %q 不应改变 webSessionMinutes，实际 %d", value, webSessionMinutes)
				}
			})
		}
	})

	t.Run("非法布尔值不开启开关", func(t *testing.T) {
		for _, value := range []string{"maybe", "2", "no-op", ""} {
			t.Run(value, func(t *testing.T) {
				restore := snapshotGlobalConfig()
				defer restore()
				resetGlobalConfigToDefaults()
				t.Setenv("CFDATA_SKIPGEO", value)

				applyEnvDefaults()

				if skipGeoCheck {
					t.Errorf("非法值 %q 不应开启 skipGeoCheck", value)
				}
			})
		}
	})

	t.Run("无法识别的调试等级不打开调试", func(t *testing.T) {
		for _, value := range []string{"verbose", "debug", "yes-please"} {
			t.Run(value, func(t *testing.T) {
				restore := snapshotGlobalConfig()
				defer restore()
				resetGlobalConfigToDefaults()
				t.Setenv("CFDATA_DEBUG", value)

				applyEnvDefaults()

				// 不能退化成 setDebugFlag 的宽松行为：一个笔误不该静默开启调试日志
				if debugMode {
					t.Errorf("无法识别的 CFDATA_DEBUG=%q 不应开启调试模式", value)
				}
			})
		}
	})
}

// 空白值等同于未设置，避免 compose 里 CFDATA_USER= 这种常见的空占位产生意外值。
func TestApplyEnvDefaultsTreatsBlankAsUnset(t *testing.T) {
	restore := snapshotGlobalConfig()
	defer restore()
	resetGlobalConfigToDefaults()

	t.Setenv("CFDATA_USER", "   ")
	t.Setenv("CFDATA_HOST", "")
	t.Setenv("CFDATA_DEBUG", "  ")

	applyEnvDefaults()

	if webUser != "" {
		t.Errorf("空白 CFDATA_USER 应视为未设置，实际 %q", webUser)
	}
	if listenHost != "" {
		t.Errorf("空 CFDATA_HOST 应视为未设置，实际 %q", listenHost)
	}
	if debugMode {
		t.Error("空白 CFDATA_DEBUG 应视为未设置，不应开启调试")
	}
}

// 会话时长的上界必须卡在 time.Duration 的溢出点之前，否则会话会静默立即失效。
func TestSessionUpperBoundPreventsDurationOverflow(t *testing.T) {
	// 用变量而非常量参与乘法：两端都是常量时 Go 会在编译期对溢出直接报错，
	// 拿不到运行期的回绕行为，也就无法验证溢出点究竟在哪。
	atBound := maxSessionMinutes
	overBound := maxSessionMinutes + 1

	if got := time.Duration(atBound) * time.Minute; got <= 0 {
		t.Fatalf("maxSessionMinutes=%d 仍会溢出，得到 %v", maxSessionMinutes, got)
	}
	if got := time.Duration(overBound) * time.Minute; got >= 0 {
		t.Fatalf("上界 +1 应已溢出（用于证明这个上界不是随手取的），实际 %v", got)
	}

	restore := snapshotGlobalConfig()
	defer restore()
	resetGlobalConfigToDefaults()
	t.Setenv("CFDATA_SESSION", "153722867")

	applyEnvDefaults()

	if webSessionMinutes != 153722867 {
		t.Errorf("上界本身应当被接受，实际 %d", webSessionMinutes)
	}
}

// TestServerFlagContract 是唯一守护「真实默认值」与「真实调用顺序」的测试。
//
// 它调用生产代码的 parseServerFlags()，不自己复刻注册与解析顺序，因此：
//   - 默认值在源码里改动会在这里失败（手抄一份默认值到测试里做不到这点）
//   - 把 parseServerFlags() 内部的 applyEnvDefaults() 挪到 flag.Parse() 之后
//     会在这里失败（命令行参数会被环境变量反向覆盖）
func TestServerFlagContract(t *testing.T) {
	parseLikeMain := func(t *testing.T, env map[string]string, args []string) {
		t.Helper()
		savedArgs, savedFlags := os.Args, flag.CommandLine
		restoreConfig := snapshotGlobalConfig()
		t.Cleanup(func() {
			os.Args, flag.CommandLine = savedArgs, savedFlags
			restoreConfig()
		})
		for name, value := range env {
			t.Setenv(name, value)
		}

		flag.CommandLine = flag.NewFlagSet("cfdata", flag.ContinueOnError)
		os.Args = append([]string{"cfdata"}, args...)

		parseServerFlags()
	}

	t.Run("默认值与源码一致", func(t *testing.T) {
		parseLikeMain(t, nil, nil)

		if listenPort != 13335 {
			t.Errorf("-port 默认值应为 13335，实际 %d", listenPort)
		}
		if listenHost != "" {
			t.Errorf("-host 默认值应为空（监听全部地址），实际 %q", listenHost)
		}
		if speedTestURL != autoSpeedURLValue {
			t.Errorf("-url 默认值应为 %q，实际 %q", autoSpeedURLValue, speedTestURL)
		}
		if skipGeoCheck {
			t.Error("-skipgeo 默认值应为 false")
		}
		if customDNSServer != defaultDNSServers {
			t.Errorf("-dns 默认值应为 %q，实际 %q", defaultDNSServers, customDNSServer)
		}
		if webUser != "" || webPassword != "" {
			t.Errorf("-user/-password 默认值应为空（不启用认证），实际 %q/%q", webUser, webPassword)
		}
		if webSessionMinutes != 720 {
			t.Errorf("-session 默认值应为 720，实际 %d", webSessionMinutes)
		}
		if debugMode {
			t.Error("-debug 默认值应为关闭")
		}
	})

	t.Run("环境变量生效", func(t *testing.T) {
		parseLikeMain(t, map[string]string{
			"CFDATA_PORT":     "8080",
			"CFDATA_HOST":     "0.0.0.0",
			"CFDATA_USER":     "admin",
			"CFDATA_PASSWORD": "s3cret",
			"CFDATA_SESSION":  "60",
			"CFDATA_SKIPGEO":  "true",
			"CFDATA_DEBUG":    "all",
		}, nil)

		if listenPort != 8080 {
			t.Errorf("环境变量应先于默认值生效，期望 8080，实际 %d", listenPort)
		}
		if listenHost != "0.0.0.0" {
			t.Errorf("期望 0.0.0.0，实际 %q", listenHost)
		}
		if webUser != "admin" || webPassword != "s3cret" {
			t.Errorf("认证信息未生效，实际 %q/%q", webUser, webPassword)
		}
		if webSessionMinutes != 60 {
			t.Errorf("期望 60，实际 %d", webSessionMinutes)
		}
		if !skipGeoCheck {
			t.Error("skipGeoCheck 未生效")
		}
		if !debugMode || debugLevel != "all" {
			t.Errorf("调试未生效，实际 mode=%v level=%q", debugMode, debugLevel)
		}
	})

	t.Run("命令行覆盖环境变量", func(t *testing.T) {
		parseLikeMain(t, map[string]string{
			"CFDATA_PORT":     "8080",
			"CFDATA_USER":     "from-env",
			"CFDATA_SKIPGEO":  "true",
			"CFDATA_SESSION":  "60",
			"CFDATA_PASSWORD": "from-env",
		}, []string{
			"-port", "9090",
			"-user", "from-cli",
			"-session", "30",
			"-skipgeo=false",
		})

		if listenPort != 9090 {
			t.Errorf("命令行应覆盖环境变量，期望 9090，实际 %d", listenPort)
		}
		if webUser != "from-cli" {
			t.Errorf("命令行应覆盖环境变量，期望 from-cli，实际 %q", webUser)
		}
		if webSessionMinutes != 30 {
			t.Errorf("命令行应覆盖环境变量，期望 30，实际 %d", webSessionMinutes)
		}
		if skipGeoCheck {
			t.Error("命令行 -skipgeo=false 应覆盖 CFDATA_SKIPGEO=true")
		}
		// 命令行没出现的项仍应保留环境变量值
		if webPassword != "from-env" {
			t.Errorf("命令行未涉及的项应保留环境变量值，期望 from-env，实际 %q", webPassword)
		}
	})

	t.Run("仅命令行时环境变量不参与", func(t *testing.T) {
		parseLikeMain(t, nil, []string{"-port", "9090"})

		if listenPort != 9090 {
			t.Errorf("期望 9090，实际 %d", listenPort)
		}
		if webSessionMinutes != 720 {
			t.Errorf("未设置的项应回落默认值 720，实际 %d", webSessionMinutes)
		}
	})
}
