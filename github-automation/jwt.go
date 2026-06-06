// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"
)

// JWTHeader represents the header of the signed JWT.
type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// JWTPayload represents the payload of the signed JWT for GitHub App authentication.
type JWTPayload struct {
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
	Iss string `json:"iss"`
}

// GenerateJWT creates a signed RS256 JWT for authenticating as a GitHub App.
func GenerateJWT(appID string, privateKeyPEM []byte) (string, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM block containing private key")
	}

	// Try standard PKCS1 (RSA Private Key)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try standard PKCS8
		key8, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return "", fmt.Errorf("failed to parse private key as PKCS1 or PKCS8: %v (PKCS1 error: %v)", err2, err)
		}
		var ok bool
		key, ok = key8.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("parsed PKCS8 key is not an RSA private key")
		}
	}

	header := JWTHeader{
		Alg: "RS256",
		Typ: "JWT",
	}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT header: %w", err)
	}

	now := time.Now().Unix()
	// GitHub recommends using a slightly past iat to avoid clock drift issues
	payload := JWTPayload{
		Iat: now - 60,
		Exp: now + 540, // 9 minutes from now (max allowed is 10 minutes)
		Iss: appID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT payload: %w", err)
	}

	b64Header := base64.RawURLEncoding.EncodeToString(headerBytes)
	b64Payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	signingInput := b64Header + "." + b64Payload
	hasher := sha256.New()
	hasher.Write([]byte(signingInput))
	hashed := hasher.Sum(nil)

	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	b64Signature := base64.RawURLEncoding.EncodeToString(signature)
	return signingInput + "." + b64Signature, nil
}
