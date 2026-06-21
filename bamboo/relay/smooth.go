package relay

import "time"

// SmoothLevel 平滑档位。
type SmoothLevel string

const (
	SmoothLevelOff        SmoothLevel = "off"
	SmoothLevelGentle     SmoothLevel = "gentle"
	SmoothLevelSmooth     SmoothLevel = "smooth"
	SmoothLevelTypewriter SmoothLevel = "typewriter"
)

// SmoothParams 平滑参数。
type SmoothParams struct {
	TokensPerFrame  int
	MinInterval     time.Duration
	MaxInterval     time.Duration
	EMAAlpha        float64
	DrainTier1Ratio float64
	DrainTier1Mult  float64
	DrainTier2Ratio float64
	DrainTier2Mult  float64
}

// SmoothConfig 平滑缓冲器配置。
type SmoothConfig struct {
	Level  SmoothLevel
	Params SmoothParams
}

// 预设档位参数表
var presetParams = map[SmoothLevel]SmoothParams{
	SmoothLevelGentle: {
		TokensPerFrame:  2,
		MinInterval:     20 * time.Millisecond,
		MaxInterval:     100 * time.Millisecond,
		EMAAlpha:        0.3,
		DrainTier1Ratio: 0.5,
		DrainTier1Mult:  0.6,
		DrainTier2Ratio: 0.2,
		DrainTier2Mult:  0.3,
	},
	SmoothLevelSmooth: {
		TokensPerFrame:  1,
		MinInterval:     15 * time.Millisecond,
		MaxInterval:     80 * time.Millisecond,
		EMAAlpha:        0.25,
		DrainTier1Ratio: 0.5,
		DrainTier1Mult:  0.5,
		DrainTier2Ratio: 0.2,
		DrainTier2Mult:  0.25,
	},
	SmoothLevelTypewriter: {
		TokensPerFrame:  1,
		MinInterval:     30 * time.Millisecond,
		MaxInterval:     120 * time.Millisecond,
		EMAAlpha:        0.2,
		DrainTier1Ratio: 0.5,
		DrainTier1Mult:  0.7,
		DrainTier2Ratio: 0.2,
		DrainTier2Mult:  0.4,
	},
}

// WithSmoothBuffer 启用平滑缓冲器（预设档位）。
//
// 支持的档位：gentle / smooth / typewriter。
// 传入未知档位时 Option 会静默跳过（保持 Smooth 为 nil）。
func WithSmoothBuffer(level SmoothLevel) Option {
	return func(c *Config) {
		params, ok := presetParams[level]
		if !ok {
			return
		}
		c.Smooth = &SmoothConfig{
			Level:  level,
			Params: params,
		}
	}
}

// WithSmoothBufferCustom 启用平滑缓冲器（自定义参数）。
//
// 档位标记为 "custom"，参数完全由调用方指定。
func WithSmoothBufferCustom(params SmoothParams) Option {
	return func(c *Config) {
		c.Smooth = &SmoothConfig{
			Level:  "custom",
			Params: params,
		}
	}
}
