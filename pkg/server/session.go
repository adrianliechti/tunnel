package server

import (
	"context"
	"net"
	"strconv"

	"golang.org/x/crypto/ssh"
)

type Session struct {
	User string

	conn *ssh.ServerConn

	BindAddr string
	BindPort int
}

func (s *Session) Open(ctx context.Context, remoteAddr string) (*SessionConn, error) {
	payload := &remoteForwardChannelData{
		DestAddr: s.BindAddr,
		DestPort: uint32(s.BindPort),
	}

	if host, portstr, err := net.SplitHostPort(remoteAddr); err == nil {
		port, _ := strconv.Atoi(portstr)

		payload.OriginAddr = host
		payload.OriginPort = uint32(port)
	}

	ch, reqs, err := s.conn.OpenChannel("forwarded-tcpip", ssh.Marshal(payload))

	if err != nil {
		return nil, err
	}

	go ssh.DiscardRequests(reqs)

	conn := &SessionConn{
		Channel: ch,
	}

	return conn, nil
}
