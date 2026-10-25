//go:build !darwin

package service

func NewSupabaseSecretStore() SecretStore {
	return EnvSecretStore{}
}
