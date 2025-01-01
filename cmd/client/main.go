package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/adrianliechti/tunnel/pkg/model"
	"golang.org/x/crypto/ssh"
)

func main() {
	var host string
	var port int

	var pass string

	flag.StringVar(&host, "host", "", "public hostname")
	flag.IntVar(&port, "port", 0, "local port to tunnel")

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

	host = parts[0]
	bootstrap := strings.Join(parts[1:], ".")

	config, err := loadConfig(bootstrap)

	if err != nil {
		panic(err)
	}

	addr := fmt.Sprintf("localhost:%d", port)

	publicURL, _ := url.Parse(config.URL)
	publicURL.Host = host + "." + publicURL.Host

	sshHost := config.SSH.Host
	sshPublicKey := mustPublicKey(config.SSH.PublicKey)

	sshConfig := &ssh.ClientConfig{
		User: "tunnel",

		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	if pass != "" {
		sshConfig.Auth = append(sshConfig.Auth, ssh.Password(pass))
	}

	if sshPublicKey != nil {
		sshConfig.HostKeyCallback = ssh.FixedHostKey(sshPublicKey)
	}

	client, err := ssh.Dial("tcp", sshHost, sshConfig)

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

	fmt.Printf("forwarding %s -> http://%s\n", publicURL, addr)

	go func() {
		for c := range client.HandleChannelOpen("forwarded-tcpip") {
			go func(c ssh.NewChannel) {
				remote, reqs, err := c.Accept()

				if err != nil {
					return
				}

				go ssh.DiscardRequests(reqs)

				local, err := net.Dial("tcp", addr)

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

func loadConfig(rawURL string) (*model.Config, error) {
	if !strings.HasPrefix(rawURL, "http") && !strings.HasPrefix(rawURL, "https") {
		rawURL = "https://" + rawURL
	}

	u, err := url.Parse(rawURL)

	if err != nil {
		return nil, err
	}

	if u.Path == "" {
		u.Path = "/config"
	}

	resp, err := http.Get(u.String())

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var config model.Config

	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func mustPublicKey(s string) ssh.PublicKey {
	data, err := base64.StdEncoding.DecodeString(s)

	if err != nil {
		panic(err)
	}

	key, err := ssh.ParsePublicKey(data)

	if err != nil {
		panic(err)
	}

	return key
}
