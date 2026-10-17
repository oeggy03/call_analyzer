//go:build darwin

package service

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

type KeychainSecretStore struct {
	Service string
	Account string
}

func NewKeychainSecretStore() *KeychainSecretStore {
	return &KeychainSecretStore{
		Service: "github.com/oeggy03.call_analyzer",
		Account: "OPENROUTER_API_KEY",
	}
}

func (s *KeychainSecretStore) Get(ctx context.Context, name string) (string, error) {
	service, account := s.values(name)
	cmd := exec.CommandContext(ctx, "security", "find-generic-password",
		"-s", service, "-a", account, "-w")
	output, err := cmd.Output()
	if err != nil {
		return "", ErrSecretNotFound
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (s *KeychainSecretStore) Set(ctx context.Context, name, value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("secret: value is required")
	}
	service, account := s.values(name)
	cmd := exec.CommandContext(ctx, "security", "add-generic-password",
		"-U", "-s", service, "-a", account, "-w", value)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.New("secret: keychain write failed")
	}
	return nil
}

func (s *KeychainSecretStore) Delete(ctx context.Context, name string) error {
	service, account := s.values(name)
	cmd := exec.CommandContext(ctx, "security", "delete-generic-password",
		"-s", service, "-a", account)
	if err := cmd.Run(); err != nil {
		// Deleting a missing item is idempotent.
		return nil
	}
	return nil
}

func (s *KeychainSecretStore) values(name string) (string, string) {
	service := s.Service
	if service == "" {
		service = "github.com/oeggy03.call_analyzer"
	}
	account := s.Account
	if account == "" {
		account = name
	}
	return service, account
}

func NewDefaultSecretStore() SecretStore {
	return ChainedSecretStore{
		Stores: []SecretStore{
			EnvSecretStore{},
			NewKeychainSecretStore(),
		},
	}
}
