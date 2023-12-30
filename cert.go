// Copyright (C) 2023 Opsmate, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.
//
// Except as contained in this notice, the name(s) of the above copyright
// holders shall not be used in advertising or otherwise to promote the
// sale, use or other dealings in this Software without prior written
// authorization.

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
