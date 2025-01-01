package server

import (
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

type Session struct {
	ID   string
	User string

	Env map[string]string
	Cmd string

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

	if host, portstr, err := net.SplitHostPort(remoteAddr); err == nil {
		port, _ := strconv.Atoi(portstr)

		payload.OriginatorAddr = host
		payload.OriginatorPort = uint32(port)
	}

	ch, reqs, err := s.conn.OpenChannel("forwarded-tcpip", ssh.Marshal(payload))

	if err != nil {
		return nil, err
	}

	go ssh.DiscardRequests(reqs)

	conn := &connectionWrapper{
		Channel: ch,
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
