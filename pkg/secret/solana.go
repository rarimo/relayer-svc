package secret

import "github.com/olegfomenko/solana-go"

type SolanaSecrets struct {
	key solana.PrivateKey
}

func NewSolanaSecrets(key solana.PrivateKey) *SolanaSecrets {
	return &SolanaSecrets{key: key}
}

func (s *SolanaSecrets) PublicKey() solana.PublicKey {
	return s.key.PublicKey()
}

func (s *SolanaSecrets) PrivateKey() solana.PrivateKey {
	return s.key
}
