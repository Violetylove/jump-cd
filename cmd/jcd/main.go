// Command jcd 是 jump-cd 的入口。
package main

import (
	"context"
	"os"

	"github.com/Violetylove/jump-cd/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:]))
}
