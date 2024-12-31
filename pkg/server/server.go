package server

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

var (
	//go:embed public
	public embed.FS
)

type Server struct {
	domain string

	sshd  *ssh.Server
	httpd *http.Server

	sessions map[string]*Session
}

func NewServer() *Server {
	domain := os.Getenv("DOMAIN")
	password := os.Getenv("PASSWORD")

	if domain == "" {
		domain = "localhost"
	}

	s := &Server{
		domain:   domain,
		sessions: make(map[string]*Session),
	}

	s.sshd = &ssh.Server{
		Addr: ":2222",

		Handler: s.handleSession,

		PasswordHandler: func(ctx ssh.Context, pass string) bool {
			if password == "" {
				return true
			}

			return password == pass
		},

		PtyCallback: func(ctx ssh.Context, pty ssh.Pty) bool {
			return false
		},

		RequestHandlers: map[string]ssh.RequestHandler{
			"tcpip-forward":        s.handleTCPForward,
			"cancel-tcpip-forward": s.handleTCPForwardCancel,
		},

		ReversePortForwardingCallback: ssh.ReversePortForwardingCallback(func(ctx ssh.Context, host string, port uint32) bool {
			return true
		}),
	}

	s.httpd = &http.Server{
		Addr: ":2280",

		Handler: s,
	}

	return s
}

func (s *Server) ListenAndServe() error {
	result := make(chan error)

	go func() {
		if err := s.httpd.ListenAndServe(); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}

			result <- err
		}
	}()

	go func() {
		if err := s.sshd.ListenAndServe(); err != nil {
			if errors.Is(err, ssh.ErrServerClosed) {
				return
			}

			result <- err
		}
	}()

	return <-result
}

func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var result error

	if err := s.httpd.Shutdown(ctx); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, err)
		}
	}

	if err := s.httpd.Close(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, err)
		}
	}

	if err := s.sshd.Shutdown(ctx); err != nil {
		if !errors.Is(err, ssh.ErrServerClosed) {
			result = errors.Join(result, err)
		}

	}

	if err := s.sshd.Close(); err != nil {
		if !errors.Is(err, ssh.ErrServerClosed) {
			result = errors.Join(result, err)
		}

	}

	return result
}

func (s *Server) handleSession(session ssh.Session) {
	sessionID := session.Context().SessionID()

	go func() {
		<-session.Context().Done()

		if session, ok := s.sessions[sessionID]; ok {
			delete(s.sessions, sessionID)

			slog.Info("session closed", "user", session.User, "bind_addr", session.BindAddr, "bind_port", session.BindPort)
		}

	}()

	for session.Context().Err() == nil {
	}
}

func (s *Server) handleTCPForward(ctx ssh.Context, srv *ssh.Server, req *gossh.Request) (bool, []byte) {
	conn := ctx.Value(ssh.ContextKeyConn).(*gossh.ServerConn)

	var payload remoteForwardRequest

	if err := gossh.Unmarshal(req.Payload, &payload); err != nil {
		return false, []byte{}
	}

	if srv.ReversePortForwardingCallback == nil || !srv.ReversePortForwardingCallback(ctx, payload.BindAddr, payload.BindPort) {
		return false, []byte("port forwarding is disabled")
	}

	addr := payload.BindAddr
	port := payload.BindPort

	if port == 0 {
		port = 80
	}

	sessionID := ctx.SessionID()

	session := &Session{
		User: conn.User(),

		BindAddr: addr,
		BindPort: int(port),

		conn: conn,
	}

	slog.Info("new session", "user", session.User, "bind_addr", session.BindAddr, "bind_port", session.BindPort)

	s.sessions[sessionID] = session

	return true, gossh.Marshal(&remoteForwardSuccess{uint32(port)})
}

func (s *Server) handleTCPForwardCancel(ctx ssh.Context, srv *ssh.Server, req *gossh.Request) (ok bool, payload []byte) {
	var reqPayload remoteForwardCancelRequest

	if err := gossh.Unmarshal(req.Payload, &reqPayload); err != nil {
		return false, []byte{}
	}

	return true, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slog.Info("http request", "host", r.Host, "method", r.Method, "url", r.RequestURI)

	host, _ := splitHostPort(r.Host)

	if strings.HasSuffix(host, "."+s.domain) {
		target, _ := url.Parse("http://" + r.Host)

		session, ok := s.sessionByHost(host)

		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		proxy := &httputil.ReverseProxy{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					slog.Info("http dial", "host", host, "addr", addr)

					conn, err := session.Open(ctx, r.RemoteAddr)

					if err != nil {
						return nil, err
					}

					return conn, nil
				},

				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},

			Rewrite: func(r *httputil.ProxyRequest) {
				r.SetURL(target)
			},
		}

		proxy.ServeHTTP(w, r)
		return
	}

	fs, _ := fs.Sub(public, "public")
	http.FileServerFS(fs).ServeHTTP(w, r)
}

func (s *Server) sessionByHost(val string) (*Session, bool) {
	host := strings.Split(val, ".")[0]

	for _, session := range s.sessions {
		addr := strings.Split(session.BindAddr, ".")[0]

		if !strings.EqualFold(host, addr) {
			continue
		}

		return session, true
	}

	return nil, false
}
