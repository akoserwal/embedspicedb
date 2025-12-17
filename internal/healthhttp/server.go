package healthhttp

import (
	"context"
	"net"
	"net/http"
	"time"
)

// Server is a small wrapper around an HTTP server + bound listener, so callers can
// safely bind to :0 and then discover the actual address via Addr().
type Server struct {
	ln   net.Listener
	srv  *http.Server
	addr string
}

// Start creates and starts an HTTP server bound to the provided address.
// The caller is responsible for calling Shutdown.
func Start(address string, handler http.Handler) (*Server, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	s := &Server{
		ln:   ln,
		addr: ln.Addr().String(),
		srv: &http.Server{
			Handler:      handler,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
	}

	go func() {
		// Serve will return http.ErrServerClosed on Shutdown; callers decide logging.
		_ = s.srv.Serve(ln)
	}()

	return s, nil
}

// Addr returns the bound address (e.g. "127.0.0.1:54321").
func (s *Server) Addr() string {
	if s == nil {
		return ""
	}
	return s.addr
}

// Shutdown stops the server and closes its listener.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.srv == nil {
		return nil
	}
	// Shutdown will stop accepting new connections and gracefully drain existing ones.
	// It also closes the listener used by Serve.
	return s.srv.Shutdown(ctx)
}
