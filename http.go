package nathttp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"

	"github.com/csbxd/natnet"
)

type Config struct {
	Network string
	NAT     natnet.Config
}

func Listen(ctx context.Context, addr string, cfg Config) (net.Listener, *natnet.Runner, error) {
	if cfg.Network == "" {
		cfg.Network = "tcp4"
	}
	ln, err := natnet.Listen(ctx, cfg.Network, addr)
	if err != nil {
		return nil, nil, err
	}
	localAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		ln.Close()
		return nil, nil, errors.New("listener address is not TCP")
	}
	cfg.NAT.Network = cfg.Network
	cfg.NAT.LocalAddr = localAddr
	r, err := natnet.Start(ctx, cfg.NAT)
	if err != nil {
		ln.Close()
		return nil, nil, err
	}
	return ln, r, nil
}

func Serve(ctx context.Context, srv *http.Server, cfg Config) error {
	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}
	ln, runner, err := Listen(ctx, addr, cfg)
	if err != nil {
		return err
	}
	return serve(ctx, srv, ln, runner, func(ln net.Listener) error {
		return srv.Serve(ln)
	})
}

func ServeTLS(ctx context.Context, srv *http.Server, cfg Config, certFile, keyFile string) error {
	addr := srv.Addr
	if addr == "" {
		addr = ":https"
	}
	ln, runner, err := Listen(ctx, addr, cfg)
	if err != nil {
		return err
	}
	return serve(ctx, srv, ln, runner, func(ln net.Listener) error {
		return srv.ServeTLS(ln, certFile, keyFile)
	})
}

func ServeTLSConfig(ctx context.Context, srv *http.Server, cfg Config, tlsConfig *tls.Config) error {
	addr := srv.Addr
	if addr == "" {
		addr = ":https"
	}
	ln, runner, err := Listen(ctx, addr, cfg)
	if err != nil {
		return err
	}
	return serve(ctx, srv, ln, runner, func(ln net.Listener) error {
		return srv.Serve(tls.NewListener(ln, tlsConfig))
	})
}

func serve(ctx context.Context, srv *http.Server, ln net.Listener, runner *natnet.Runner, fn func(net.Listener) error) error {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			ln.Close()
		case <-done:
		}
	}()

	err := fn(ln)
	close(done)
	runnerErr := runner.Close()
	if errors.Is(err, net.ErrClosed) || errors.Is(err, http.ErrServerClosed) {
		err = ctx.Err()
	}
	if err != nil {
		return err
	}
	return runnerErr
}
