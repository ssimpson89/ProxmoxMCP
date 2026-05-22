package proxmox

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	px "github.com/luthermonson/go-proxmox"

	"github.com/ssimpson/ProxmoxMCP/config"
)

func NewClient(cfg *config.Config) (*px.Client, error) {
	transport := &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          10,
	}

	if !cfg.VerifyTLS {
		fmt.Fprintln(os.Stderr, "WARNING: TLS certificate verification is disabled")
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   2 * time.Minute,
	}

	baseURL := fmt.Sprintf("%s/api2/json", cfg.Host)

	opts := []px.Option{
		px.WithHTTPClient(httpClient),
	}

	if cfg.UseTokenAuth() {
		opts = append(opts, px.WithAPIToken(cfg.TokenID, cfg.TokenSecret))
	} else {
		opts = append(opts, px.WithCredentials(&px.Credentials{
			Username: cfg.Username,
			Password: cfg.Password,
		}))
	}

	client := px.NewClient(baseURL, opts...)

	return client, nil
}

func ValidateConnection(ctx context.Context, client *px.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := client.Version(ctx)
	if err != nil {
		return fmt.Errorf("cannot reach Proxmox API: %v", err)
	}
	return nil
}
