package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"sync"
	"time"
)

var selfSignedKey crypto.Signer
var selfSignedKeyGen sync.Once

func generateRandomSerialNumber() *big.Int {
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(err)
	}
	return serialNumber
}

func getSelfSignedCert(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	selfSignedKeyGen.Do(func() {
		if key, err := rsa.GenerateKey(rand.Reader, 2048); err == nil {
			selfSignedKey = key
		}
	})
	if selfSignedKey == nil {
		return nil, fmt.Errorf("generating key failed")
	}
	template := &x509.Certificate{
		DNSNames:     []string{hello.ServerName},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		NotAfter:     time.Now().Add(12 * time.Hour),
		NotBefore:    time.Now().Add(-12 * time.Hour),
		SerialNumber: generateRandomSerialNumber(),
		Subject:      pkix.Name{CommonName: hello.ServerName},
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, selfSignedKey.Public(), selfSignedKey)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  selfSignedKey,
	}, nil
}
