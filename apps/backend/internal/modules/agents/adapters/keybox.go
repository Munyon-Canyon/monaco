package adapters

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"io"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/domain"
)

const keyBytes = 32

type KeyBox struct {
	aead cipher.AEAD
}

func NewKeyBox(base64Key string) (KeyBox, error) {
	const op = "agents.NewKeyBox"
	raw, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return KeyBox{}, errs.Wrap(err, errs.CodeInvalidConfig, op)
	}
	var aead cipher.AEAD
	block, err := aes.NewCipher(raw)
	if err == nil {
		aead, err = cipher.NewGCM(block)
	}
	if err != nil || len(raw) != keyBytes {
		return KeyBox{}, errs.Wrap(err, errs.CodeInvalidConfig, op)
	}
	return KeyBox{aead: aead}, nil
}

func (b KeyBox) Seal(random io.Reader, key domain.Key) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "agents.KeyBox.Seal")
	}
	return b.aead.Seal(nonce, nonce, []byte(key.String()), nil), nil
}

func (b KeyBox) Open(sealed []byte) (domain.Key, error) {
	const op = "agents.KeyBox.Open"
	nonceSize := b.aead.NonceSize()
	if len(sealed) < nonceSize+b.aead.Overhead() {
		return domain.Key{}, errs.New(errs.CodeDecodeFailed, op)
	}
	plain, err := b.aead.Open(nil, sealed[:nonceSize], sealed[nonceSize:], nil)
	if err != nil {
		return domain.Key{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	key, err := domain.ParseKey(string(plain))
	if err != nil {
		return domain.Key{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return key, nil
}
