package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/felipemarinho97/godeez/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd.RootCmd.ExecuteContext(ctx)
}
