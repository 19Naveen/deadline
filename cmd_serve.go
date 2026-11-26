package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gotodo/internal/web"
)

// runServe serves a read-only HTML board on the loopback interface until
// Ctrl+C. Non-loopback addresses are refused.
func runServe(path string, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:7777", "loopback address to listen on")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (usage: gotodo serve [-addr 127.0.0.1:7777])", fs.Arg(0))
	}
	if err := web.ValidateAddr(*addr); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &web.Server{Path: path}
	err := web.Serve(ctx, *addr, srv.Handler(), func(url string) {
		fmt.Printf("serving read-only board at %s (Ctrl+C to stop)\n", url)
	})
	if err == nil {
		fmt.Println("stopped")
	}
	return err
}
