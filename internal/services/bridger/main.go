package bridger

import (
	"fmt"
	"sync"

	"github.com/rarimo/relayer-svc/internal/config"
	"github.com/rarimo/relayer-svc/internal/services/bridger/bridge"
	"github.com/rarimo/relayer-svc/internal/services/bridger/evm"
	"github.com/rarimo/relayer-svc/internal/services/bridger/solana"
	"github.com/rarimo/relayer-svc/internal/types"
)

type BridgerProvider interface {
	// GetBridger returns the bridger for the given chain
	GetBridger(chain string) bridge.Bridger
}

type bridgerProvider struct {
	cfg      config.Config
	bridgers map[string]bridge.Bridger
	mu       sync.Mutex
}

func NewBridgerProvider(cfg config.Config) BridgerProvider {
	return &bridgerProvider{
		bridgers: make(map[string]bridge.Bridger),
		cfg:      cfg,
	}
}

func (p *bridgerProvider) GetBridger(chain string) bridge.Bridger {
	p.mu.Lock()
	defer p.mu.Unlock()

	if bridger, ok := p.bridgers[chain]; ok {
		return bridger
	}

	var bridger bridge.Bridger
	switch {
	case types.IsEVM(chain):
		bridger = evm.NewEVMBridger(p.cfg)
	case chain == types.Solana:
		bridger = solana.NewSolanaBridger(p.cfg)
	default:
		panic(fmt.Errorf("unknown chain %s", chain))
	}
	p.bridgers[chain] = bridger

	return bridger
}
