// Package version 保存构建期注入的版本信息。
package version

// 这些变量由 goreleaser 或 go build -ldflags 注入。
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
