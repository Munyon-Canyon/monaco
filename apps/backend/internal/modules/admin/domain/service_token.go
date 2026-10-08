package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"regexp"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	ServiceTokenPrefix = "mst_"
	serviceTokenBytes  = 32
)

var serviceTokenName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

func ParseServiceTokenName(raw string) (string, error) {
	if !serviceTokenName.MatchString(raw) {
		return "", errs.New(errs.CodeInvalidInput, "admin.ParseServiceTokenName")
	}
	return raw, nil
}

func IsServiceToken(raw string) bool { return strings.HasPrefix(raw, ServiceTokenPrefix) }

func HashServiceToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func NewServiceToken(random io.Reader) (token string, hash []byte, err error) {
	var buf [serviceTokenBytes]byte
	if _, err = io.ReadFull(random, buf[:]); err != nil {
		return "", nil, errs.Wrap(err, errs.CodeInternal, "admin.NewServiceToken")
	}
	token = ServiceTokenPrefix + base64.RawURLEncoding.EncodeToString(buf[:])
	return token, HashServiceToken(token), nil
}
