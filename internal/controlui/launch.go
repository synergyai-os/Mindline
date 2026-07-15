package controlui

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
)

type BrowserOpener interface {
	Open(string) error
}

type RunningServer struct {
	Listener net.Listener
	HTTP     *http.Server
	URL      string
}

func Launch(ctx context.Context, app Application, runtimeRoot string, opener BrowserOpener) (*RunningServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	host := listener.Addr().String()
	server, err := New(app, Options{ExpectedHost: host, Origin: "http://" + host, RuntimeRoot: runtimeRoot})
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	httpServer := HTTPServer(listener, server.Handler())
	running := &RunningServer{Listener: listener, HTTP: httpServer, URL: "http://" + host + "/#" + server.BootstrapFragment()}
	serveErr := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()
	if opener != nil {
		if err := opener.Open(running.URL); err != nil {
			_ = httpServer.Shutdown(context.Background())
			return nil, err
		}
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = httpServer.Shutdown(context.Background())
		case <-serveErr:
		}
	}()
	return running, nil
}

func (s *RunningServer) Shutdown(ctx context.Context) error {
	if s == nil || s.HTTP == nil {
		return nil
	}
	return s.HTTP.Shutdown(ctx)
}

func (s *RunningServer) SafeDisplayURL() string {
	if s == nil {
		return ""
	}
	return strings.SplitN(s.URL, "#", 2)[0]
}
