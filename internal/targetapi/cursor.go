package targetapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/zrma/sema/internal/repository"
)

const cursorSchemaVersion = 1

type cursorBinding struct {
	Tenant       string
	ResourceKind string
	Filter       string
	Order        string
}

type cursorPosition struct {
	RepositoryVersion repository.Version
	After             string
}

type cursorPayload struct {
	SchemaVersion     int                `json:"v"`
	RepositoryVersion repository.Version `json:"repository_version"`
	After             string             `json:"after"`
}

type cursorCodec struct {
	key []byte
}

func newCursorCodec(key []byte) (cursorCodec, error) {
	if len(key) < 32 {
		return cursorCodec{}, fmt.Errorf("cursor authentication key must contain at least 32 bytes")
	}
	return cursorCodec{key: append([]byte(nil), key...)}, nil
}

func (codec cursorCodec) encode(binding cursorBinding, position cursorPosition) (string, error) {
	payload, err := json.Marshal(cursorPayload{
		SchemaVersion: cursorSchemaVersion, RepositoryVersion: position.RepositoryVersion,
		After: position.After,
	})
	if err != nil {
		return "", fmt.Errorf("encode cursor payload: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := codec.sign(binding, encoded)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (codec cursorCodec) decode(token string, binding cursorBinding) (cursorPosition, error) {
	if token == "" || len(token) > 2048 {
		return cursorPosition{}, errors.New("cursor is empty or too large")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return cursorPosition{}, errors.New("cursor has an invalid format")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, codec.sign(binding, parts[0])) {
		return cursorPosition{}, errors.New("cursor authentication failed")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return cursorPosition{}, errors.New("cursor payload is not base64url")
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	var decoded cursorPayload
	if err := decoder.Decode(&decoded); err != nil {
		return cursorPosition{}, errors.New("cursor payload is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return cursorPosition{}, errors.New("cursor payload has trailing data")
	}
	if decoded.SchemaVersion != cursorSchemaVersion || decoded.RepositoryVersion == 0 || decoded.After == "" {
		return cursorPosition{}, errors.New("cursor payload is incomplete")
	}
	return cursorPosition{
		RepositoryVersion: decoded.RepositoryVersion, After: decoded.After,
	}, nil
}

func (codec cursorCodec) sign(binding cursorBinding, encodedPayload string) []byte {
	mac := hmac.New(sha256.New, codec.key)
	_, _ = mac.Write([]byte(binding.Tenant))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(binding.ResourceKind))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(binding.Filter))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(binding.Order))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}
