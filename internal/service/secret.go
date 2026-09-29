package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
)

var ErrSecretNotFound = errors.New("secret: not found")

type SecretStore interface {
	Get(context.Context, string) (string, error)
	Set(context.Context, string, string) error
	Delete(context.Context, string) error
}

type MemorySecretStore struct {
	mu     sync.RWMutex
	values map[string]string
}

func NewMemorySecretStore() *MemorySecretStore {
	return &MemorySecretStore{values: make(map[string]string)}
}

func (s *MemorySecretStore) Get(ctx context.Context, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[name]
	if !ok || strings.TrimSpace(value) == "" {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (s *MemorySecretStore) Set(ctx context.Context, name, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("secret: name is required")
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("secret: value is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[name] = value
	return nil
}

func (s *MemorySecretStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, name)
	return nil
}

type EnvSecretStore struct {
	Prefix string
}

func (s EnvSecretStore) Get(ctx context.Context, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(name) == "" {
		return "", ErrSecretNotFound
	}
	envName := name
	if s.Prefix != "" {
		envName = strings.TrimRight(s.Prefix, "_") + "_" + envName
	}
	envName = strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(envName))
	value, ok := os.LookupEnv(envName)
	if !ok || strings.TrimSpace(value) == "" {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (s EnvSecretStore) Set(context.Context, string, string) error {
	return errors.New("secret: environment store is read-only")
}

func (s EnvSecretStore) Delete(context.Context, string) error {
	return errors.New("secret: environment store is read-only")
}

type ChainedSecretStore struct {
	Stores []SecretStore
}

func (s ChainedSecretStore) Get(ctx context.Context, name string) (string, error) {
	for _, store := range s.Stores {
		if store == nil {
			continue
		}
		value, err := store.Get(ctx, name)
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, ErrSecretNotFound) {
			return "", err
		}
	}
	return "", ErrSecretNotFound
}

func (s ChainedSecretStore) Set(ctx context.Context, name, value string) error {
	for _, store := range s.Stores {
		if store == nil {
			continue
		}
		if err := store.Set(ctx, name, value); err == nil {
			return nil
		}
	}
	return errors.New("secret: no writable store")
}

func (s ChainedSecretStore) Delete(ctx context.Context, name string) error {
	for _, store := range s.Stores {
		if store == nil {
			continue
		}
		if err := store.Delete(ctx, name); err == nil {
			return nil
		}
	}
	return errors.New("secret: no writable store")
}

type APIKeySecretProvider struct {
	Store SecretStore
	Name  string
}

func (p APIKeySecretProvider) APIKey(ctx context.Context) (string, error) {
	if p.Store == nil {
		return "", ErrSecretNotFound
	}
	name := p.Name
	if name == "" {
		name = "OPENROUTER_API_KEY"
	}
	return p.Store.Get(ctx, name)
}
