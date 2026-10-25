//go:build darwin

package service

func NewSupabaseSecretStore() SecretStore {
	return ChainedSecretStore{
		Stores: []SecretStore{
			EnvSecretStore{},
			&KeychainSecretStore{
				Service: "github.com/oeggy03.call_analyzer",
				Account: "SUPABASE_SECRET_KEY",
			},
		},
	}
}
