
## Example

```
$ python3 -m http.server
$ ssh -T -p 2222 -R test:0:127.0.0.1:8000 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ServerAliveInterval=30 localhost
```