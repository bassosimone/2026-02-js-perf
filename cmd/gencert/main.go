// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/bassosimone/pkitest"
	"github.com/bassosimone/runtimex"
	"github.com/bassosimone/vclip"
	"github.com/bassosimone/vflag"
)

func main() {
	vclip.Main(context.Background(), vclip.CommandFunc(run), os.Args[1:])
}

func run(ctx context.Context, args []string) error {
	var (
		outputDir = "./testdata"
		ipAddr    = "127.0.0.1"
	)

	fset := vflag.NewFlagSet("gencert", vflag.ExitOnError)
	fset.AutoHelp('h', "help", "Print this help text and exit.")
	fset.StringVar(&ipAddr, 0, "ip-addr", "Use `ADDR` as an IP SAN.")
	fset.StringVar(&outputDir, 'o', "output-dir", "Write certificates to `DIR`.")
	runtimex.PanicOnError0(fset.Parse(args))

	ip := net.ParseIP(ipAddr)
	if ip == nil {
		log.Fatalf("gencert: invalid IP address: %s", ipAddr)
	}

	// Check whether existing certificates are still valid for this IP.
	certPath := filepath.Join(outputDir, "cert.pem")
	if existingCertIsValid(certPath, ip) {
		log.Printf("gencert: certificates are valid, nothing to do")
		return nil
	}

	config := &pkitest.SelfSignedCertConfig{
		CommonName:   ipAddr,
		DNSNames:     []string{ipAddr},
		IPAddrs:      []net.IP{ip},
		Organization: []string{"ocho"},
	}

	runtimex.LogFatalOnError0(os.MkdirAll(outputDir, 0700))
	pkitest.MustNewSelfSignedCert(config).MustWriteFiles(outputDir)

	log.Printf("gencert: wrote %s", filepath.Join(outputDir, "cert.pem"))
	log.Printf("gencert: wrote %s", filepath.Join(outputDir, "key.pem"))
	return nil
}

// existingCertIsValid returns true if the cert at certPath exists,
// does not expire within 30 days, and contains the given IP SAN.
func existingCertIsValid(certPath string, wantIP net.IP) bool {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	if time.Until(cert.NotAfter) < 30*24*time.Hour {
		return false
	}
	for _, ip := range cert.IPAddresses {
		if ip.Equal(wantIP) {
			return true
		}
	}
	return false
}
