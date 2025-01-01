package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"strings"

	"golang.org/x/crypto/ssh"
)

func main() {
	var host string
	var port int

	var user string
	var pass string

	flag.StringVar(&host, "host", "", "public hostname")
	flag.IntVar(&port, "port", 0, "local port to tunnel")

	flag.StringVar(&user, "user", "tunnel", "username")
	flag.StringVar(&pass, "pass", "", "password")

	flag.Parse()

	if host == "" {
		panic("host is required")
	}

	if port == 0 {
		panic("port is required")
	}

	parts := strings.Split(host, ".")

	if len(parts) < 3 {
		panic("invalid host")
	}

	server := strings.Join(parts[1:], ".") + ":2222"
	target := fmt.Sprintf("localhost:%d", port)

	config := &ssh.ClientConfig{
		User: user,

		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},

		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", server, config)

	if err != nil {
		panic(err)
	}

	defer client.Close()

	forwardMessage := struct {
		BindAddr string
		BindPort uint32
	}{
		BindAddr: host,
		BindPort: 0,
	}

	ok, _, err := client.SendRequest("tcpip-forward", true, ssh.Marshal(&forwardMessage))

	if !ok || err != nil {
		panic("failed to forward port")
	}

	fmt.Printf("forwarding https://%s -> http://%s\n", host, target)

	go func() {
		for c := range client.HandleChannelOpen("forwarded-tcpip") {
			go func(c ssh.NewChannel) {
				remote, reqs, err := c.Accept()

				if err != nil {
					return
				}

				go ssh.DiscardRequests(reqs)

				local, err := net.Dial("tcp", target)

				if err != nil {
					return
				}

				go io.Copy(local, remote)
				io.Copy(remote, local)
			}(c)
		}
	}()

	client.Wait()
}
