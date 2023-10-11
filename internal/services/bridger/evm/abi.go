package evm

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

var bytes32SliceType = mustABIType("bytes32[]")
var bytesType = mustABIType("bytes")

var proofABI = abi.Arguments{{Type: bytes32SliceType}, {Type: bytesType}}

func mustABIType(evmType string) abi.Type {
	abiType, err := abi.NewType(evmType, "", nil)
	if err != nil {
		panic(errors.Wrap(err, "failed to register ABI type"))
	}

	return abiType
}
