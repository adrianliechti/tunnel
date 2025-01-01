package server

import (
	"net"
	"net/netip"
	"time"

	"golang.org/x/crypto/ssh"
)

type Session struct {
	ID   string
	User string

	Env  map[string]string
	Cmd  string
	Args []string

	BindAddr string
	BindPort uint32

	conn *ssh.ServerConn
}

func (s *Session) Dial(remoteAddr string) (net.Conn, error) {
	payload := struct {
		ConnectedAddr string
		ConnectedPort uint32

		OriginatorAddr string
		OriginatorPort uint32
	}{
		ConnectedAddr: s.BindAddr,
		ConnectedPort: s.BindPort,
	}

	var remote net.Addr

	if val, err := netip.ParseAddrPort(remoteAddr); err == nil {
		payload.OriginatorAddr = val.Addr().String()
		payload.OriginatorPort = uint32(val.Port())

		remote = net.TCPAddrFromAddrPort(val)
	}

	ch, reqs, err := s.conn.OpenChannel("forwarded-tcpip", ssh.Marshal(payload))

	if err != nil {
		return nil, err
	}

	go ssh.DiscardRequests(reqs)

	conn := &connectionWrapper{
		Channel: ch,

		remote: remote,
	}

	return conn, nil
}

func (s *Session) Exit(code int) error {
	// https://datatracker.ietf.org/doc/html/rfc4254#section-6.10
	message := struct {
		Status uint32
	}{
		uint32(code),
	}

	_, _, err := s.conn.SendRequest("exit-status", false, ssh.Marshal(&message))

	if err != nil {
		return err
	}

	return s.conn.Close()
}

func (s *Session) Arg(name string) string {
	for i := 0; i < len(s.Args); i++ {
		if s.Args[i] == "-"+name && len(s.Args) > i {
			return s.Args[i+1]
		}
	}

	return ""
}

type connectionWrapper struct {
	ssh.Channel

	local  net.Addr
	remote net.Addr
}

func (c *connectionWrapper) LocalAddr() net.Addr {
	return c.local
}

func (c *connectionWrapper) RemoteAddr() net.Addr {
	return c.remote
}

func (c *connectionWrapper) SetDeadline(t time.Time) error {
	return nil
}

func (c *connectionWrapper) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *connectionWrapper) SetWriteDeadline(t time.Time) error {
	return nil
}
