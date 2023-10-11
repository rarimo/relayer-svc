package core

import (
	rarimocore "github.com/rarimo/rarimo-core/x/rarimocore/types"
	tokenmanager "github.com/rarimo/rarimo-core/x/tokenmanager/types"
)

type TransferDetails struct {
	Transfer       rarimocore.Transfer
	Collection     tokenmanager.Collection
	CollectionData tokenmanager.CollectionData
	Item           tokenmanager.Item
	Signature      string
	Origin         string
	MerklePath     [][32]byte
}
