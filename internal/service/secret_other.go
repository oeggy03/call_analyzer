//go:build !darwin

package service

func NewDefaultSecretStore() SecretStore {
	return ChainedSecretStore{
		Stores: []SecretStore{
			EnvSecretStore{},
			NewMemorySecretStore(),
		},
	}
}
