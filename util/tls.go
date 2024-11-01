// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

// Modified from: golang/src/crypto/tls/generate_cert.go

package util

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

// NewTLS builds a new TLS struct with some defaults.
func NewTLS() *TLS {
	return &TLS{
		ValidFor:     365 * 24 * time.Hour,
		RsaBits:      2048,
		Organization: "Example Corp",
	}
}

// TLS handles all of the TLS building.
type TLS struct {
	// Host is comma-separated hostnames and IPs to generate a certificate
	// for.
	Host string

	// ValidFrom is the creation date formatted as Jan 1 15:04:05 2011.
	ValidFrom string

	// ValidFor is the duration that certificate is valid for.
	ValidFor time.Duration

	// IsCA is Whether this cert should be its own Certificate Authority.
	IsCA bool

	// RsaBits is the size of RSA key to generate. Ignored if EcdsaCurve is
	// set.
	RsaBits int

	// EcdsaCurve is the ECDSA curve to use to generate a key. Valid values
	// are P224, P256 (recommended), P384, P521.
	EcdsaCurve string

	// Ed25519Key is true to generate an Ed25519 key.
	Ed25519Key bool

	// Organization is the name of the organization to store in the cert.
	Organization string
}

// Generate writes out the two files. Usually keyPemFile is key.pem and
// certPemFile is cert.pem which go into http.ListenAndServeTLS.
func (obj *TLS) Generate(keyPemFile, certPemFile string) error {
	if len(obj.Host) == 0 {
		return fmt.Errorf("missing required Host parameter")
	}

	var priv any
	var err error
	switch obj.EcdsaCurve {
	case "":
		if obj.Ed25519Key {
			_, priv, err = ed25519.GenerateKey(rand.Reader)
		} else {
			priv, err = rsa.GenerateKey(rand.Reader, obj.RsaBits)
		}
	case "P224":
		priv, err = ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	case "P256":
		priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "P384":
		priv, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "P521":
		priv, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	default:
		return fmt.Errorf("unrecognized elliptic curve: %q", obj.EcdsaCurve)
	}
	if err != nil {
		return fmt.Errorf("failed to generate private key: %v", err)
	}

	// ECDSA, ED25519 and RSA subject keys should have the DigitalSignature
	// KeyUsage bits set in the x509.Certificate template
	keyUsage := x509.KeyUsageDigitalSignature
	// Only RSA subject keys should have the KeyEncipherment KeyUsage bits set. In
	// the context of TLS this KeyUsage is particular to RSA key exchange and
	// authentication.
	if _, isRSA := priv.(*rsa.PrivateKey); isRSA {
		keyUsage |= x509.KeyUsageKeyEncipherment
	}

	var notBefore time.Time
	if len(obj.ValidFrom) == 0 {
		notBefore = time.Now()
	} else {
		notBefore, err = time.Parse("Jan 2 15:04:05 2006", obj.ValidFrom)
		if err != nil {
			return fmt.Errorf("failed to parse creation date: %v", err)
		}
	}

	notAfter := notBefore.Add(obj.ValidFor)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{obj.Organization},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              keyUsage,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	hosts := strings.Split(obj.Host, ",")
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	if obj.IsCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, obj.publicKey(priv), priv)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %v", err)
	}

	certOut, err := os.Create(certPemFile)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %v", certPemFile, err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to write data to %s: %v", certPemFile, err)
	}
	if err := certOut.Close(); err != nil {
		return fmt.Errorf("error closing %s: %v", certPemFile, err)
	}
	log.Printf("wrote %s", certPemFile)

	keyOut, err := os.OpenFile(keyPemFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %v", keyPemFile, err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("unable to marshal private key: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return fmt.Errorf("failed to write data to %s: %v", keyPemFile, err)
	}
	if err := keyOut.Close(); err != nil {
		return fmt.Errorf("error closing %s: %v", keyPemFile, err)
	}
	log.Printf("wrote %s", keyPemFile)

	return nil
}

func (obj *TLS) publicKey(priv any) any {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	case ed25519.PrivateKey:
		return k.Public().(ed25519.PublicKey)
	default:
		return nil
	}
}
