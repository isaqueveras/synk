package contract

import "github.com/isaqueveras/synk"

type ContractServiceArgs struct {
	ContractArgs
}

func NewService(id, name string) synk.JobArgs {
	return &ContractArgs{
		CustomerID:   id,
		CustomerName: name,
	}
}
