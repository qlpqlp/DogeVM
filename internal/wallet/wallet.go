// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"golang.org/x/crypto/pbkdf2"
)

const (
	ExpectedChainID         = "2hFCfzdMmfXBxYgvvdL7BYiJAxdejyn4AksMYUM2eM5gN7Xrjy"
	ExpectedDogecoinVMNet   = "dogecoinvm"
	pbkdf2Iterations        = 120_000
	encryptedBlobVersion    = 1
)

var (
	ErrLocked      = errors.New("wallet is locked")
	ErrNoWallet    = errors.New("no wallet on disk")
	ErrBadPass     = errors.New("incorrect passphrase")
	ErrExists      = errors.New("wallet already exists")
)

// Store holds an encrypted Dogecoin key on disk and an optional unlocked session.
type Store struct {
	mu       sync.RWMutex
	path     string
	unlocked *Session
}

// Session is an unlocked key in memory (never sent to remote APIs).
type Session struct {
	PrivateKey *btcec.PrivateKey
	Address    string
	Hash160    []byte
	CreatedAt  time.Time
}

type diskBlob struct {
	Version    int    `json:"version"`
	Address    string `json:"address"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
	CreatedAt  string `json:"createdAt"`
}

func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dataDir, "wallet.json")}, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

func (s *Store) PublicInfo() (address string, createdAt string, exists bool, unlocked bool, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	unlocked = s.unlocked != nil
	if s.unlocked != nil {
		return s.unlocked.Address, s.unlocked.CreatedAt.UTC().Format(time.RFC3339), true, true, nil
	}
	blob, err := s.readBlob()
	if err != nil {
		if errors.Is(err, ErrNoWallet) {
			return "", "", false, false, nil
		}
		return "", "", false, false, err
	}
	return blob.Address, blob.CreatedAt, true, false, nil
}

func (s *Store) Session() (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unlocked == nil {
		return nil, ErrLocked
	}
	return s.unlocked, nil
}

func (s *Store) Create(passphrase string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.path); err == nil {
		return nil, ErrExists
	}
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}
	sess, err := sessionFromKey(priv)
	if err != nil {
		return nil, err
	}
	if err := s.writeEncrypted(passphrase, sess); err != nil {
		return nil, err
	}
	s.unlocked = sess
	return sess, nil
}

func (s *Store) ImportWIF(wifStr, passphrase string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.path); err == nil {
		return nil, ErrExists
	}
	wif, err := btcutil.DecodeWIF(wifStr)
	if err != nil {
		return nil, fmt.Errorf("decode WIF: %w", err)
	}
	sess, err := sessionFromKey(wif.PrivKey)
	if err != nil {
		return nil, err
	}
	if err := s.writeEncrypted(passphrase, sess); err != nil {
		return nil, err
	}
	s.unlocked = sess
	return sess, nil
}

func (s *Store) ImportHex(hexKey, passphrase string) (*Session, error) {
	raw, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}
	if len(raw) != 32 {
		return nil, errors.New("private key must be 32 bytes")
	}
	priv, _ := btcec.PrivKeyFromBytes(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.path); err == nil {
		return nil, ErrExists
	}
	sess, err := sessionFromKey(priv)
	if err != nil {
		return nil, err
	}
	if err := s.writeEncrypted(passphrase, sess); err != nil {
		return nil, err
	}
	s.unlocked = sess
	return sess, nil
}

func (s *Store) Unlock(passphrase string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	blob, err := s.readBlob()
	if err != nil {
		return nil, err
	}
	salt, err := hex.DecodeString(blob.Salt)
	if err != nil {
		return nil, err
	}
	nonce, err := hex.DecodeString(blob.Nonce)
	if err != nil {
		return nil, err
	}
	ct, err := hex.DecodeString(blob.Ciphertext)
	if err != nil {
		return nil, err
	}
	key := pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrBadPass
	}
	if len(plain) != 32 {
		return nil, errors.New("corrupt wallet blob")
	}
	priv, _ := btcec.PrivKeyFromBytes(plain)
	sess, err := sessionFromKey(priv)
	if err != nil {
		return nil, err
	}
	if created, err := time.Parse(time.RFC3339, blob.CreatedAt); err == nil {
		sess.CreatedAt = created
	}
	if sess.Address != blob.Address {
		return nil, errors.New("wallet address mismatch")
	}
	s.unlocked = sess
	return sess, nil
}

func (s *Store) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unlocked = nil
}

func (s *Store) ExportWIF() (string, error) {
	sess, err := s.Session()
	if err != nil {
		return "", err
	}
	wif, err := btcutil.NewWIF(sess.PrivateKey, &DogeMainNetParams, true)
	if err != nil {
		return "", err
	}
	return wif.String(), nil
}

func (s *Store) ExportHex() (string, error) {
	sess, err := s.Session()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sess.PrivateKey.Serialize()), nil
}

func (s *Store) Remove() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unlocked = nil
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func sessionFromKey(priv *btcec.PrivateKey) (*Session, error) {
	pub := priv.PubKey()
	addr, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(pub.SerializeCompressed()), &DogeMainNetParams)
	if err != nil {
		return nil, err
	}
	return &Session{
		PrivateKey: priv,
		Address:    addr.EncodeAddress(),
		Hash160:    addr.ScriptAddress(),
		CreatedAt:  time.Now().UTC(),
	}, nil
}

func (s *Store) writeEncrypted(passphrase string, sess *Session) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key := pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iterations, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, sess.PrivateKey.Serialize(), nil)
	blob := diskBlob{
		Version:    encryptedBlobVersion,
		Address:    sess.Address,
		Salt:       hex.EncodeToString(salt),
		Nonce:      hex.EncodeToString(nonce),
		Ciphertext: hex.EncodeToString(ct),
		CreatedAt:  sess.CreatedAt.UTC().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(blob, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) readBlob() (*diskBlob, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoWallet
		}
		return nil, err
	}
	var blob diskBlob
	if err := json.Unmarshal(raw, &blob); err != nil {
		return nil, err
	}
	return &blob, nil
}

// AddressHash160 returns the 20-byte hash160 for a Dogecoin base58 address.
func AddressHash160(addr string) ([]byte, byte, error) {
	a, err := btcutil.DecodeAddress(addr, &DogeMainNetParams)
	if err != nil {
		return nil, 0, err
	}
	switch v := a.(type) {
	case *btcutil.AddressPubKeyHash:
		return v.ScriptAddress(), 0x00, nil
	case *btcutil.AddressScriptHash:
		return v.ScriptAddress(), 0x01, nil
	default:
		return nil, 0, errors.New("unsupported address type (need P2PKH or P2SH)")
	}
}
