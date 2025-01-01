package server

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/adrianliechti/tunnel/pkg/model"
)

var (
	//go:embed public
	public embed.FS
)

type Server struct {
	domain string

	sshPort      int
	sshConfig    *ssh.ServerConfig
	sshPublicKey []byte

	httpPort int

	sessions map[string]*Session
}

func NewServer() (*Server, error) {
	domain := os.Getenv("DOMAIN")
	password := os.Getenv("PASSWORD")

	if domain == "" {
		domain = "localhost"
	}

	sshConfig := &ssh.ServerConfig{
		NoClientAuth: true,
	}

	if password != "" {
		sshConfig.NoClientAuth = false

		sshConfig.PasswordCallback = func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) != password {
				return nil, fmt.Errorf("password rejected for %q", conn.User())
			}

			return &ssh.Permissions{}, nil
		}
	}

	hostKey, err := ReadHostKey("")

	if err != nil {
		return nil, err
	}

	sshConfig.AddHostKey(hostKey)

	return &Server{
		domain: domain,

		sshPort:      2222,
		sshConfig:    sshConfig,
		sshPublicKey: hostKey.PublicKey().Marshal(),

		httpPort: 2280,

		sessions: make(map[string]*Session),
	}, nil
}

func (s *Server) ListenAndServe() error {
	sshAddr := fmt.Sprintf(":%d", s.sshPort)
	httpAddr := fmt.Sprintf(":%d", s.httpPort)

	sshListener, err := net.Listen("tcp", sshAddr)

	if err != nil {
		return err
	}

	httpListener, err := net.Listen("tcp", httpAddr)

	if err != nil {
		return err
	}

	go http.Serve(httpListener, s)

	for {
		c, err := sshListener.Accept()

		if err != nil {
			continue
		}

		go s.handleConnection(c)
	}
}

func (s *Server) handleConnection(c net.Conn) {
	conn, chans, reqs, err := ssh.NewServerConn(c, s.sshConfig)

	if err != nil {
		return
	}

	session := &Session{
		conn: conn,

		ID:   hex.EncodeToString(conn.SessionID()),
		User: conn.User(),

		Env: make(map[string]string),
	}

	s.sessions[session.ID] = session

	slog.Debug("connection", "session", session.ID, "user", conn.User(), "remote_addr", conn.RemoteAddr())

	go func() {
		for req := range reqs {
			switch req.Type {
			case "tcpip-forward":
				s.handleTCPForward(session, req)

			case "cancel-tcpip-forward":
				s.cancelTCPForward(session, req)

			case "keepalive@openssh.com":
				if req.WantReply {
					req.Reply(true, nil)
				}

			default:
				slog.Debug("reject request", "type", req.Type)

				if req.WantReply {
					req.Reply(false, nil)
				}
			}
		}
	}()

	go func() {
		for ch := range chans {
			switch ch.ChannelType() {
			case "session":
				go s.handleSession(session, ch)

			default:
				slog.Debug("reject channel", "type", ch.ChannelType())
				ch.Reject(ssh.UnknownChannelType, "unknown channel type")
			}
		}
	}()

	conn.Wait()

	delete(s.sessions, session.ID)
}

func (s *Server) handleSession(session *Session, newChan ssh.NewChannel) {
	ch, reqs, err := newChan.Accept()

	if err != nil {
		return
	}

	_ = ch

	for req := range reqs {
		switch req.Type {
		case "pty-req":
			// https://datatracker.ietf.org/doc/html/rfc4254#section-6.2
			var payload struct {
				Term string

				Width uint32
				Heigh uint32

				WindowWidth  uint32
				WindowHeight uint32

				TermModes string
			}

			ssh.Unmarshal(req.Payload, &payload)

			if req.WantReply {
				req.Reply(false, nil)
			}

		case "env":
			// https://datatracker.ietf.org/doc/html/rfc4254#section-6.4
			var payload struct {
				Key   string
				Value string
			}

			ssh.Unmarshal(req.Payload, &payload)
			slog.Debug("env", "session", session.ID, "key", payload.Key, "value", payload.Value)

			if payload.Key != "" {
				session.Env[payload.Key] = payload.Value
			}

			if req.WantReply {
				req.Reply(true, nil)
			}

		case "shell", "exec":
			// https://datatracker.ietf.org/doc/html/rfc4254#section-6.5
			var payload struct {
				Command string
			}

			ssh.Unmarshal(req.Payload, &payload)
			slog.Debug("exec", "session", session.ID, "command", payload.Command)

			session.Cmd = payload.Command

			if req.WantReply {
				req.Reply(true, nil)
			}

		case "window-change":
			// https://datatracker.ietf.org/doc/html/rfc4254#section-6.7
			var payload struct {
				Width uint32
				Heigh uint32

				WindowWidth  uint32
				WindowHeight uint32
			}

			ssh.Unmarshal(req.Payload, &payload)

			if req.WantReply {
				req.Reply(false, nil)
			}

		default:
			slog.Debug("reject session request", "type", req.Type)

			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

func (s *Server) handleTCPForward(session *Session, req *ssh.Request) {
	// https://datatracker.ietf.org/doc/html/rfc4254#section-7.1
	var payload struct {
		Addr string
		Port uint32
	}

	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		if req.WantReply {
			req.Reply(false, nil)
		}

		return
	}

	var result []byte

	if payload.Port == 0 {
		payload.Port = 80

		// https://datatracker.ietf.org/doc/html/rfc4254#section-7.1
		message := struct {
			Port uint32
		}{
			payload.Port,
		}

		result = ssh.Marshal(&message)
	}

	session.BindAddr = payload.Addr
	session.BindPort = payload.Port

	if req.WantReply {
		req.Reply(true, result)
	}
}

func (s *Server) cancelTCPForward(session *Session, req *ssh.Request) {
	// https://datatracker.ietf.org/doc/html/rfc4254#section-7.1
	var payload struct {
		Addr string
		Port uint32
	}

	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		if req.WantReply {
			req.Reply(false, nil)
		}

		return
	}

	if req.WantReply {
		req.Reply(true, nil)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slog.Info("request", "host", r.Host, "method", r.Method, "url", r.RequestURI)

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
					slog.Debug("dial", "host", host, "addr", addr)

					conn, err := session.Dial(r.RemoteAddr)

					if err != nil {
						return nil, err
					}

					return conn, nil
				},

				ForceAttemptHTTP2: true,

				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,

				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},

			Rewrite: func(r *httputil.ProxyRequest) {
				r.SetURL(target)
				r.SetXForwarded()
			},
		}

		proxy.ServeHTTP(w, r)
		return
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		data := model.Config{
			URL: fmt.Sprintf("https://%s", s.domain),

			SSH: &model.SSHConfig{
				Host: fmt.Sprintf("%s:%d", s.domain, s.sshPort),

				PublicKey: base64.StdEncoding.EncodeToString(s.sshPublicKey),
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fs, _ := fs.Sub(public, "public")
		http.FileServerFS(fs).ServeHTTP(w, r)
	})

	mux.ServeHTTP(w, r)
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
