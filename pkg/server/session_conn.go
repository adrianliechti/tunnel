package server

import (
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	_ net.Conn = &SessionConn{}
)

type SessionConn struct {
	ssh.Channel

	local  net.Addr
	remote net.Addr
}

func (conn *SessionConn) LocalAddr() net.Addr {
	return conn.local
}

func (conn *SessionConn) RemoteAddr() net.Addr {
	return conn.remote
}

func (conn *SessionConn) SetDeadline(t time.Time) error {
	return nil
}

func (conn *SessionConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (conn *SessionConn) SetWriteDeadline(t time.Time) error {
	return nil
}
