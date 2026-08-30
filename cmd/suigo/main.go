// suigo は開発用 CLI。v0.1 では `suigo dev` のみを提供する。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "dev" {
		fmt.Fprintln(os.Stderr, "usage: suigo dev [-port N] [-watch DIR] <package>")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("dev", flag.ExitOnError)
	port := fs.Int("port", 7373, "TCP port to listen on (127.0.0.1 only)")
	watch := fs.String("watch", ".", "directory to watch for Go source changes")
	metrics := fs.Bool("metrics", false, "print reload timing (p50) on exit")
	_ = fs.Parse(os.Args[2:])
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: suigo dev [-port N] [-watch DIR] <package>")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d := &devServer{port: *port, pkg: fs.Arg(0), watchDir: *watch, metrics: *metrics}
	if err := d.run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "suigo dev:", err)
		os.Exit(1)
	}
}
