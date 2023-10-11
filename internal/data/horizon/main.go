package horizon

type Horizon interface {
	NftMetadata(chain, tokenIndex, id string) (*NftMetadata, error)
}
