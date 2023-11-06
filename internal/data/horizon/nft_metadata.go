package horizon

import (
	horizonresources "github.com/rarimo/horizon-svc/resources"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"net/http"
	"path"
)

type NftMetadata horizonresources.NftMetadataAttributes

func (h *horizon) NftMetadata(chain, tokenIndex, id string) (*NftMetadata, error) {
	resp := new(horizonresources.NftMetadataResponse)
	endpoint := path.Join("/v1/items/", tokenIndex, "/chains/", chain, "/nfts/", id, "/metadata")
	if err := h.do(http.MethodGet, endpoint, resp); err != nil {
		return nil, errors.Wrap(err, "failed to get nft metadata")
	}

	return (*NftMetadata)(&resp.Data.Attributes), nil
}
