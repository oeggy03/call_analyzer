package service

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

func TestChainedDeleteClearsEveryStore(t *testing.T) {
	ctx := context.Background()
	first := NewMemorySecretStore()
	second := NewMemorySecretStore()
	if err := first.Set(ctx, "key", "first"); err != nil {
		t.Fatal(err)
	}
	if err := second.Set(ctx, "key", "fallback"); err != nil {
		t.Fatal(err)
	}
	chain := ChainedSecretStore{Stores: []SecretStore{first, second}}
	if err := chain.Delete(ctx, "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Get(ctx, "key"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("primary store still contains key: %v", err)
	}
	if _, err := second.Get(ctx, "key"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("fallback store still contains key: %v", err)
	}
}

func TestDefaultSecretStoreHasNoUnexpectedInMemoryFallback(t *testing.T) {
	chain, ok := NewDefaultSecretStore().(ChainedSecretStore)
	if !ok {
		t.Fatalf("default secret store type = %T", NewDefaultSecretStore())
	}
	if len(chain.Stores) != 2 {
		t.Fatalf("default secret store has %d stores, want two platform-safe stores", len(chain.Stores))
	}
	if runtime.GOOS == "darwin" {
		for _, store := range chain.Stores {
			if _, ok := store.(*MemorySecretStore); ok {
				t.Fatal("macOS default secret chain contains an in-memory fallback")
			}
		}
	}
}
