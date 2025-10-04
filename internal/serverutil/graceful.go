package serverutil

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// SetupGracefulShutdown installs a signal waiter that shuts down the server gracefully.
func SetupGracefulShutdown(h *server.Hertz, timeout time.Duration, cleanup func(context.Context) error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	h.SetCustomSignalWaiter(func(errCh chan error) error {
		sigCh := make(chan os.Signal, 2)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		sig := <-sigCh
		log.Printf("received signal %s, shutting down", sig)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		var result error

		if err := h.Engine.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
			result = err
		}

		if cleanup != nil {
			if err := cleanup(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- err
				if result == nil {
					result = err
				}
			}
		}

		select {
		case sig = <-sigCh:
			log.Printf("received signal %s during shutdown, forcing exit", sig)
			return errors.New("forced exit")
		default:
		}

		return result
	})
}
