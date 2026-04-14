package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

type tailscaleLocalStatus struct {
	BackendState string   `json:"BackendState"`
	AuthURL      string   `json:"AuthURL"`
	Health       []string `json:"Health"`
	Self         *struct {
		ID           string   `json:"ID"`
		HostName     string   `json:"HostName"`
		DNSName      string   `json:"DNSName"`
		Online       bool     `json:"Online"`
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Self"`
	Peer map[string]*tailscalePeerStatus `json:"Peer"`
}

type tailscalePeerStatus struct {
	ID             string   `json:"ID"`
	HostName       string   `json:"HostName"`
	DNSName        string   `json:"DNSName"`
	Online         bool     `json:"Online"`
	TailscaleIPs   []string `json:"TailscaleIPs"`
	OS             string   `json:"OS"`
	ExitNodeOption bool     `json:"ExitNodeOption"`
	Active         bool     `json:"Active"`
}

func readTailscaleLocalStatus(ctx context.Context, socketPath string) (tailscaleLocalStatus, error) {
	return readTailscaleLocalStatusWithPeers(ctx, socketPath, false)
}

func readTailscaleLocalStatusWithPeers(ctx context.Context, socketPath string, includePeers bool) (tailscaleLocalStatus, error) {
	if _, err := os.Stat(socketPath); err != nil {
		return tailscaleLocalStatus{}, err
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   3 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://local-tailscaled.sock/localapi/v0/status?peers=%t", includePeers), nil)
	if err != nil {
		return tailscaleLocalStatus{}, err
	}
	//nolint:gosec // Requests are sent to a fixed local API endpoint over a unix domain socket.
	resp, err := client.Do(req)
	if err != nil {
		return tailscaleLocalStatus{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tailscaleLocalStatus{}, fmt.Errorf("tailscale localapi status: unexpected status %d", resp.StatusCode)
	}
	var status tailscaleLocalStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return tailscaleLocalStatus{}, err
	}
	return status, nil
}
