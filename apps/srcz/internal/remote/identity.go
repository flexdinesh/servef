// Package remote is what serving over TCP adds to the web app (decision 0098):
// the server's identity, its TLS listener, authentication, compression, and the
// addresses a client might reach it at. It knows no daemon, CLI, or web routes.
package remote

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/flexdinesh/srcz/pkg/connection"
)

const (
	identityVersion = 1
	// certificateLifetime is long on purpose: the pin, not the dates, is what
	// clients trust, and an expiry would break saved connections.
	certificateLifetime = 10 * 365 * 24 * time.Hour
)

// Identity is who a remote server is: its certificate and the token clients
// must present. It persists until rotated, so restarts keep saved connections
// and printed links working.
type Identity struct {
	Certificate tls.Certificate
	Credential  connection.Credential
	// Port is the port it last served on, tried first next time so saved
	// connections survive restarts.
	Port int
	path string
}

type identityDocument struct {
	Version     int    `json:"version"`
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
	Token       []byte `json:"token"`
	Port        int    `json:"port,omitempty"`
}

// DefaultIdentityFile is os.UserConfigDir()/srcz/remote/identity.json.
func DefaultIdentityFile() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config: %w", err)
	}
	return filepath.Join(directory, "srcz", "remote", "identity.json"), nil
}

// LoadOrCreateIdentity reads the identity at path, creating it when missing. A
// file others can read is refused: its secrets can no longer be trusted, and
// silently tightening it would hide that.
func LoadOrCreateIdentity(path string) (*Identity, error) {
	identity, err := loadIdentity(path)
	if errors.Is(err, fs.ErrNotExist) {
		return RotateIdentity(path)
	}
	return identity, err
}

func loadIdentity(path string) (*Identity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("identity file %s has mode %o; others may have read it. Run `srcz remote rotate`, then share the new connection string", path, info.Mode().Perm())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read identity: %w", err)
	}
	var document identityDocument
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("identity file %s is malformed: %w", path, err)
	}
	if document.Version != identityVersion {
		return nil, fmt.Errorf("identity file %s has version %d; this srcz reads version %d", path, document.Version, identityVersion)
	}
	certificate, err := tls.X509KeyPair([]byte(document.Certificate), []byte(document.Key))
	if err != nil {
		return nil, fmt.Errorf("identity file %s: %w", path, err)
	}
	if len(document.Token) != connection.TokenSize {
		return nil, fmt.Errorf("identity file %s has a token of %d bytes", path, len(document.Token))
	}
	return newIdentity(path, certificate, [connection.TokenSize]byte(document.Token), document.Port)
}

func newIdentity(path string, certificate tls.Certificate, token [connection.TokenSize]byte, port int) (*Identity, error) {
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("read certificate: %w", err)
	}
	return &Identity{
		Certificate: certificate,
		Credential:  connection.Credential{Fingerprint: connection.FingerprintOf(leaf.RawSubjectPublicKeyInfo), Token: token},
		Port:        port,
		path:        path,
	}, nil
}

// RotateIdentity replaces the identity at path with a new key and token, which
// invalidates every connection string made from the old one. It keeps the port.
func RotateIdentity(path string) (*Identity, error) {
	port := 0
	if old, err := loadIdentity(path); err == nil {
		port = old.Port
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("create key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "srcz"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(certificateLifetime),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"srcz"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("encode key: %w", err)
	}
	var token [connection.TokenSize]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := writeIdentity(path, identityDocument{Version: identityVersion, Certificate: string(certificatePEM), Key: string(keyPEM), Token: token[:], Port: port}); err != nil {
		return nil, err
	}
	certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("read new certificate: %w", err)
	}
	return newIdentity(path, certificate, token, port)
}

// RememberPort records the port this identity serves on.
func (i *Identity) RememberPort(port int) error {
	if i.Port == port {
		return nil
	}
	content, err := os.ReadFile(i.path)
	if err != nil {
		return fmt.Errorf("read identity: %w", err)
	}
	var document identityDocument
	if err := json.Unmarshal(content, &document); err != nil {
		return fmt.Errorf("identity file %s is malformed: %w", i.path, err)
	}
	document.Port = port
	if err := writeIdentity(i.path, document); err != nil {
		return err
	}
	i.Port = port
	return nil
}

// writeIdentity writes atomically through a 0600 temporary file, in a 0700
// directory, so no reader sees a partial file or a moment of wider access.
func writeIdentity(path string, document identityDocument) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create identity directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("restrict identity directory: %w", err)
	}
	content, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".identity-*")
	if err != nil {
		return fmt.Errorf("write identity: %w", err)
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	if _, err := temporary.Write(append(content, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write identity: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("write identity: %w", err)
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return fmt.Errorf("write identity: %w", err)
	}
	return nil
}

// TLSConfig is the listener's configuration. TLS 1.3 only: every client is
// this build's Go proxy or a current browser.
func (i *Identity) TLSConfig() *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{i.Certificate}, MinVersion: tls.VersionTLS13}
}
