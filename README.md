# Tunnel

Tunnel allows to expose a local web server to a public http/https endpoint.

## Setup

### Requirements

- a (sub-)domain
- configured `@` A-record (e.g. `tunnel.ws`)
- configured `*` A-record (e.g. `*.tunnel.ws`)
- a reverse proxy to handle TLS (e.g. Caddy, Nginx, ...)
- an open tcp port for SSH connections (e.g. 2222)

Generate SSH host key:

```
ssh-keygen -t rsa -b 4096 -f id_rsa
```

Sample [Caddy](https://caddyserver.com) configuration:

```
tunnel.ws {
    reverse_proxy tunnel:2280
}

*.tunnel.ws {
    reverse_proxy tunnel:2280

    # see https://caddyserver.com/docs/automatic-https#dns-challenge
	tls {
		dns godaddy {env.GODADDY_TOKEN}
	}
}
```

## Example

Using Tunnel Client:

```bash
client -host test.tunnel.ws -port 5173 [ -pass s3cr3t ]
```

Using SSH Client

```bash
ssh -N -R test:0:127.0.0.1:5173 -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ServerAliveInterval=30 tunnel.ws
```