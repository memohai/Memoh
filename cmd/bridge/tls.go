package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"google.golang.org/grpc/credentials"
)

// bridge TCP 通道的 strict mTLS（设计 memoh-saas-bridge-mtls-design.md §8.1）。
// 材料由 memoh-bridge-mtls Secret 挂载，env 只传路径与模式，私钥不进 env。
const (
	bridgeTLSModeEnv              = "BRIDGE_TLS_MODE"
	bridgeTLSCertFileEnv          = "BRIDGE_TLS_CERT_FILE"
	bridgeTLSKeyFileEnv           = "BRIDGE_TLS_KEY_FILE"
	bridgeTLSClientCAFileEnv      = "BRIDGE_TLS_CLIENT_CA_FILE"
	bridgeTLSExpectedClientURIEnv = "BRIDGE_TLS_EXPECTED_CLIENT_URI"

	bridgeTLSModeDisabled = "disabled"
	bridgeTLSModeStrict   = "strict"
)

// bridgeServerCredentials 按 BRIDGE_TLS_MODE 构建 gRPC server credentials。
// disabled/空 → (nil, nil) 维持现状；strict → 必须 mTLS，材料缺失即错误，
// 绝不静默回退明文（设计 §10）。仅对 TCP listener 调用；UDS 走文件系统权限。
func bridgeServerCredentials() (credentials.TransportCredentials, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(bridgeTLSModeEnv)))
	switch mode {
	case "", bridgeTLSModeDisabled:
		return nil, nil
	case bridgeTLSModeStrict:
	default:
		return nil, fmt.Errorf("unknown %s %q (want %s|%s)", bridgeTLSModeEnv, mode, bridgeTLSModeDisabled, bridgeTLSModeStrict)
	}

	certFile := strings.TrimSpace(os.Getenv(bridgeTLSCertFileEnv))
	keyFile := strings.TrimSpace(os.Getenv(bridgeTLSKeyFileEnv))
	caFile := strings.TrimSpace(os.Getenv(bridgeTLSClientCAFileEnv))
	expectedURI := strings.TrimSpace(os.Getenv(bridgeTLSExpectedClientURIEnv))
	if certFile == "" || keyFile == "" || caFile == "" || expectedURI == "" {
		return nil, fmt.Errorf("strict bridge TLS requires %s, %s, %s and %s", bridgeTLSCertFileEnv, bridgeTLSKeyFileEnv, bridgeTLSClientCAFileEnv, bridgeTLSExpectedClientURIEnv)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load bridge server keypair: %w", err)
	}
	caPEM, err := os.ReadFile(caFile) //nolint:gosec // G304: path comes from operator-controlled env, not end-user input
	if err != nil {
		return nil, fmt.Errorf("read server client CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no certificates parsed from %s", caFile)
	}

	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		// RequireAndVerifyClientCert 已验链（含 ClientAuth EKU）；这里把调用方
		// 钉死到本 instance 的 Memoh Server SPIFFE 身份。被攻破的 bot 只持有
		// shared bridge server cert（ServerAuth、bridge URI），过不了这一关。
		VerifyConnection: func(cs tls.ConnectionState) error {
			return verifyMemohServerClientIdentity(cs, expectedURI)
		},
	}
	return credentials.NewTLS(cfg), nil
}

func verifyMemohServerClientIdentity(cs tls.ConnectionState, expectedURI string) error {
	if len(cs.PeerCertificates) == 0 {
		return errors.New("client certificate required")
	}
	leaf := cs.PeerCertificates[0]
	if !slices.Contains(leaf.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
		return errors.New("client certificate lacks ClientAuth EKU")
	}
	for _, uri := range leaf.URIs {
		if uri.String() == expectedURI {
			return nil
		}
	}
	return fmt.Errorf("client certificate URI SAN mismatch (want %s)", expectedURI)
}
