package provider

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// SDKName SDK 名称标识，用于生成统一 User-Agent 字符串的前缀。
const SDKName = "BM-SDK"

// modulePath 模块路径，用于在 build info 的依赖列表中定位自身模块。
const modulePath = "github.com/bamboo-services/bamboo-messages"

// userAgent 缓存的 User-Agent 字符串，通过 sync.Once 保证只初始化一次。
var (
	userAgent     string
	userAgentOnce sync.Once
)

// sdkVersion 缓存的 SDK 版本号，通过 sync.Once 保证只初始化一次。
var (
	sdkVersion     string
	sdkVersionOnce sync.Once
)

// GetUserAgent 生成统一的 User-Agent 字符串。
//
// 格式为 "BM-SDK/{version}"，版本号通过 runtime/debug.ReadBuildInfo() 动态读取。
// 优先使用 info.Main.Version，回退到依赖列表中查找自身模块，最终回退到 "dev"。
// 通过 sync.Once 保证并发安全且只初始化一次。
func GetUserAgent() string {
	userAgentOnce.Do(func() {
		version := GetSDKVersion()
		userAgent = fmt.Sprintf("%s/%s", SDKName, version)
	})
	return userAgent
}

// GetSDKVersion 获取 SDK 版本号。
//
// 通过 runtime/debug.ReadBuildInfo() 从编译后的二进制中读取版本信息：
//  1. 优先检查 info.Main.Version（非空且非 "(devel)" 时使用）
//  2. 回退到遍历 info.Deps，查找模块路径匹配的依赖版本
//  3. 最终回退到 "dev"（本地开发环境）
//
// 通过 sync.Once 保证并发安全且只初始化一次。
func GetSDKVersion() string {
	sdkVersionOnce.Do(func() {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			sdkVersion = "dev"
			return
		}

		// 优先使用 Main.Version
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			sdkVersion = info.Main.Version
			return
		}

		// 回退到依赖列表中查找
		for _, dep := range info.Deps {
			if dep.Path == modulePath {
				if dep.Version != "" {
					sdkVersion = dep.Version
					return
				}
			}
		}

		// 最终回退
		sdkVersion = "dev"
	})
	return sdkVersion
}