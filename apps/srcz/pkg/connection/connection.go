// Package connection names srcz servers for the CLI and the desktop app
// (decision 0098): endpoints, the connection string, how to reach an endpoint,
// and the saved connections. It knows no CLI, UI, or server internals.
//
// A connection string says where a remote server is and how to trust it:
//
//	<host>[,<host>...]:<port>/<credential>
//
// The credential is base64url of a certificate fingerprint and a token. Every
// character is safe unquoted in POSIX shells, zsh, and fish, except the
// brackets of an IPv6 host; Shell quotes those.
package connection

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

const (
	// FingerprintSize is the bytes of SHA-256 of a certificate's public key
	// kept in a credential.
	FingerprintSize = 16
	// TokenSize is the bytes of a credential's bearer token.
	TokenSize = 16
	// Local is the connection id of the local daemon.
	Local = "local"
	// maxHosts bounds the candidate addresses in a string.
	maxHosts = 8
	// maxHostLength is the longest DNS name.
	maxHostLength = 253
)

// ErrInvalid reports text that is not a connection string.
var ErrInvalid = errors.New("not a srcz connection string")

var (
	hostName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)
	idText   = regexp.MustCompile(`^r-[0-9a-f]{12}$`)
)

// Credential authenticates both ends: the fingerprint pins the server and the
// token authorizes the client.
type Credential struct {
	Fingerprint [FingerprintSize]byte
	Token       [TokenSize]byte
}

// FingerprintOf is the fingerprint of a certificate's SubjectPublicKeyInfo.
func FingerprintOf(publicKeyInfo []byte) [FingerprintSize]byte {
	sum := sha256.Sum256(publicKeyInfo)
	var fingerprint [FingerprintSize]byte
	copy(fingerprint[:], sum[:])
	return fingerprint
}

// Encode is the credential's text form.
func (c Credential) Encode() string {
	raw := make([]byte, 0, FingerprintSize+TokenSize)
	raw = append(raw, c.Fingerprint[:]...)
	raw = append(raw, c.Token[:]...)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// ParseCredential reads Encode's text.
func ParseCredential(text string) (Credential, error) {
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil || len(raw) != FingerprintSize+TokenSize {
		return Credential{}, ErrInvalid
	}
	var credential Credential
	copy(credential.Fingerprint[:], raw[:FingerprintSize])
	copy(credential.Token[:], raw[FingerprintSize:])
	return credential, nil
}

// TokenText is the bearer token as sent on the wire.
func (c Credential) TokenText() string {
	return base64.RawURLEncoding.EncodeToString(c.Token[:])
}

// ID names the server: it follows from the fingerprint alone, so it holds no
// secret, can appear in links, and stays the same across restarts and
// addresses.
func (c Credential) ID() string {
	return "r-" + hex.EncodeToString(c.Fingerprint[:6])
}

// IsID reports whether text is a remote connection id.
func IsID(text string) bool { return idText.MatchString(text) }

// Parse reads a connection string, tolerating surrounding quotes and the
// whitespace a terminal's line wrapping can add.
func Parse(text string) (TLS, error) {
	compact := strings.Join(strings.Fields(text), "")
	compact = strings.Trim(compact, `'"`)
	address, encoded, found := strings.Cut(compact, "/")
	if !found {
		return TLS{}, ErrInvalid
	}
	credential, err := ParseCredential(encoded)
	if err != nil {
		return TLS{}, err
	}
	separator := strings.LastIndex(address, ":")
	if separator < 0 {
		return TLS{}, ErrInvalid
	}
	port, err := strconv.Atoi(address[separator+1:])
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != address[separator+1:] {
		return TLS{}, ErrInvalid
	}
	var hosts []string
	for _, host := range strings.Split(address[:separator], ",") {
		host, err := parseHost(host)
		if err != nil {
			return TLS{}, err
		}
		hosts = append(hosts, host)
	}
	if len(hosts) > maxHosts {
		return TLS{}, ErrInvalid
	}
	return TLS{Hosts: hosts, Port: port, Credential: credential}, nil
}

// ParseHost validates one host of a connection string: a DNS name, an IPv4
// address, or an IPv6 address with or without brackets.
func ParseHost(text string) (string, error) { return parseHost(text) }

func parseHost(text string) (string, error) {
	if inner, ok := strings.CutPrefix(text, "["); ok {
		inner, ok = strings.CutSuffix(inner, "]")
		if !ok || !isIPv6(inner) {
			return "", ErrInvalid
		}
		return inner, nil
	}
	if isIPv6(text) {
		return text, nil
	}
	if len(text) > maxHostLength || !hostName.MatchString(text) {
		return "", ErrInvalid
	}
	return text, nil
}

func isIPv6(text string) bool {
	ip := net.ParseIP(text)
	return ip != nil && ip.To4() == nil
}

// Format is the endpoint's connection string.
func (t TLS) Format() string {
	hosts := make([]string, len(t.Hosts))
	for i, host := range t.Hosts {
		hosts[i] = host
		if isIPv6(host) {
			hosts[i] = "[" + host + "]"
		}
	}
	return strings.Join(hosts, ",") + ":" + strconv.Itoa(t.Port) + "/" + t.Credential.Encode()
}

// Shell is Format as a single shell word: quoted when brackets would otherwise
// glob.
func (t TLS) Shell() string {
	formatted := t.Format()
	if strings.Contains(formatted, "[") {
		return "'" + formatted + "'"
	}
	return formatted
}

// WebURL is the browser's address: its token is swapped for a cookie and
// stripped from the URL on arrival.
func (t TLS) WebURL() (string, error) {
	if len(t.Hosts) == 0 {
		return "", fmt.Errorf("%w: no host", ErrInvalid)
	}
	return "https://" + net.JoinHostPort(t.Hosts[0], strconv.Itoa(t.Port)) + "/?token=" + t.Credential.TokenText(), nil
}
