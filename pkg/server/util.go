package server

import (
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

func ReadHostKey(name string) (ssh.Signer, error) {
	if name == "" {
		name = "id_rsa"
	}

	data, err := os.ReadFile(name)

	if err != nil {
		return nil, err
	}

	return ssh.ParsePrivateKey(data)
}

func splitHostPort(hostPort string) (host, port string) {
	host = hostPort

	colon := strings.LastIndexByte(host, ':')
	if colon != -1 {
		host, port = host[:colon], host[colon+1:]
	}

	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}

	return
}
