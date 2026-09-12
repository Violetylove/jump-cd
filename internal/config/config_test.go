package config_test

import (
	"testing"

	"github.com/Violetylove/jump-cd/internal/config"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv(config.EnvHalfLife, "")
	t.Setenv(config.EnvTau, "")

	cfg := config.Load()
	if cfg.HalfLifeDays != config.DefaultHalfLifeDays {
		t.Fatalf("HalfLifeDays = %v", cfg.HalfLifeDays)
	}
	if cfg.AmbiguousTau != config.DefaultAmbiguousTau {
		t.Fatalf("AmbiguousTau = %v", cfg.AmbiguousTau)
	}
}

// 一个跳目录的工具不该因为环境变量写错就罢工。
func TestLoadFallsBackOnGarbage(t *testing.T) {
	for _, bad := range []string{"abc", "-1", "0", " "} {
		t.Setenv(config.EnvHalfLife, bad)
		if got := config.Load().HalfLifeDays; got != config.DefaultHalfLifeDays {
			t.Fatalf("HalfLifeDays(%q) = %v，应退回默认值", bad, got)
		}
	}
}

func TestLoadHonoursEnv(t *testing.T) {
	t.Setenv(config.EnvHalfLife, "3.5")
	t.Setenv(config.EnvTau, "2")

	cfg := config.Load()
	if cfg.HalfLifeDays != 3.5 {
		t.Fatalf("HalfLifeDays = %v", cfg.HalfLifeDays)
	}
	if cfg.AmbiguousTau != 2 {
		t.Fatalf("AmbiguousTau = %v", cfg.AmbiguousTau)
	}
}

func TestDataPathsFollowEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.EnvDataDir, dir)

	got, err := config.DataDir()
	if err != nil || got != dir {
		t.Fatalf("DataDir = %q, %v", got, err)
	}
	if p, err := config.StorePath(); err != nil || parentOf(p) != dir {
		t.Fatalf("StorePath = %q, %v", p, err)
	}
	if p, err := config.JournalPath(); err != nil || parentOf(p) != dir {
		t.Fatalf("JournalPath = %q, %v", p, err)
	}
}

func parentOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return ""
}
