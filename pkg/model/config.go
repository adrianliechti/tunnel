package model

type Config struct {
	URL string `json:"url"`

	SSH *SSHConfig `json:"ssh,omitempty"`
}

type SSHConfig struct {
	Host string `json:"host"`

	PublicKey string `json:"publicKey,omitempty"`
}
