
## Setup

Generate SSH host key:

```
ssh-keygen -t rsa -b 4096 -f id_rsa
```

Sample Caddy configuration:

```
tunnel.ws {
    reverse_proxy tunnel:2280
}

*.tunnel.ws {
    reverse_proxy tunnel:2280

	tls {
		dns godaddy {env.GODADDY_TOKEN}
	}
}
```

## Example

```
$ python3 -m http.server
$ ssh -N -v -p 2222 -R test:0:127.0.0.1:8000 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ServerAliveInterval=30 tunnel.ws
```