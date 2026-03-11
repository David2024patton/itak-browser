// Package browser - session persistence.
//
// What: Saves and restores browser state (cookies, localStorage) to/from disk.
// Why:  Agents need to resume authenticated sessions across CLI invocations
//       without re-logging in each time.
// How:  We export cookies via CDP Network.getCookies, serialize to JSON,
//       and encrypt with AES-256-GCM using a per-profile key.
package browser

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// cookieFile is the filename within the profile dir for persisted cookies.
const cookieFile = "session_cookies.enc"

// SaveSession persists the current page cookies to disk.
//
// What: Exports all cookies for the active origin.
// Why:  Allows subsequent CLI invocations to resume authenticated sessions.
// How:  Calls CDP Network.getCookies, marshals to JSON, encrypts with AES-GCM.
func (e *Engine) SaveSession(passphrase string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var cookies []*network.Cookie
	if err := chromedp.Run(e.browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	); err != nil {
		return fmt.Errorf("save session: get cookies: %w", err)
	}

	data, err := json.Marshal(cookies)
	if err != nil {
		return fmt.Errorf("save session: marshal: %w", err)
	}

	encrypted, err := encryptAESGCM(data, passphrase)
	if err != nil {
		return fmt.Errorf("save session: encrypt: %w", err)
	}

	path := filepath.Join(e.profileDir, cookieFile)
	return os.WriteFile(path, encrypted, 0600)
}

// RestoreSession loads persisted cookies from disk and injects them into Chrome.
func (e *Engine) RestoreSession(passphrase string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	path := filepath.Join(e.profileDir, cookieFile)
	encrypted, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("restore session: read file: %w", err)
	}

	data, err := decryptAESGCM(encrypted, passphrase)
	if err != nil {
		return fmt.Errorf("restore session: decrypt: %w", err)
	}

	var cookies []*network.Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return fmt.Errorf("restore session: unmarshal: %w", err)
	}

	return chromedp.Run(e.browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			for _, c := range cookies {
				if err := network.SetCookie(c.Name, c.Value).
					WithDomain(c.Domain).
					WithPath(c.Path).
					WithSecure(c.Secure).
					WithHTTPOnly(c.HTTPOnly).
					Do(ctx); err != nil {
					// Non-fatal: skip bad cookies and continue.
					e.logger.Warn("restore cookie failed", "name", c.Name, "err", err)
				}
			}
			return nil
		}),
	)
}

// encryptAESGCM encrypts plaintext using AES-256-GCM with a passphrase-derived key.
func encryptAESGCM(plaintext []byte, passphrase string) ([]byte, error) {
	key := deriveKey(passphrase)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decryptAESGCM decrypts AES-256-GCM ciphertext with a passphrase-derived key.
func decryptAESGCM(ciphertext []byte, passphrase string) ([]byte, error) {
	key := deriveKey(passphrase)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return aesGCM.Open(nil, nonce, ciphertext, nil)
}

// deriveKey produces a 32-byte AES key from a passphrase using SHA-256.
// Why SHA-256: Simple, dependency-free. A production system should use Argon2id.
func deriveKey(passphrase string) []byte {
	h := sha256.Sum256([]byte("itak-browser-v1:" + passphrase))
	return h[:]
}
