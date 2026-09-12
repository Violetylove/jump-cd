// Package config 只有两个旋钮，而且都从环境变量来。
//
// 刻意不做配置文件：一个只干一件事的小工具，多一个要发现、要解析、要出错的文件，
// 换来的只是「少改一次代码」。这两个旋钮几乎没人动，环境变量足够，
// 还省掉一个 TOML 依赖。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// appDir 是各平台下的应用目录名。
const appDir = "jump-cd"

// 环境变量名。
const (
	EnvDataDir  = "JCD_DATA_DIR"
	EnvHalfLife = "JCD_HALF_LIFE_DAYS"
	EnvTau      = "JCD_AMBIGUOUS_TAU"
)

// 默认值。
const (
	// DefaultHalfLifeDays 是记忆的半衰期：越久没去的地方，权重衰减越快。
	DefaultHalfLifeDays = 7.0
	// DefaultAmbiguousTau 是歧义阈值：前两名分数比值低于它就交给人挑，不猜。
	DefaultAmbiguousTau = 1.25
)

// Config 是全部可调项。
type Config struct {
	HalfLifeDays float64
	AmbiguousTau float64
}

// Load 读取配置。任何一项缺失或非法都会安静地退回默认值——
// 一个跳目录的工具不该因为环境变量写错就罢工。
func Load() Config {
	return Config{
		HalfLifeDays: positiveFloat(EnvHalfLife, DefaultHalfLifeDays),
		AmbiguousTau: positiveFloat(EnvTau, DefaultAmbiguousTau),
	}
}

func positiveFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return fallback
	}
	return f
}

// DataDir 返回数据目录。
func DataDir() (string, error) {
	if v := os.Getenv(EnvDataDir); v != "" {
		return v, nil
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, appDir), nil
		}
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, appDir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: 找不到用户主目录: %w", err)
	}
	return filepath.Join(home, ".local", "share", appDir), nil
}

// StorePath 返回目录历史文件的位置。
func StorePath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "dirs.json"), nil
}

// JournalPath 返回 shell 追加日志的位置。
func JournalPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "journal"), nil
}
