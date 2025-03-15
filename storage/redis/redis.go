package redis

import (
	"github.com/isaqueveras/synk/storage"
	"github.com/isaqueveras/synk/types"
)

type repository struct{}

func NewStorage() storage.Storage {
	return &repository{}
}

func (r *repository) GetJobAvailable(queue string, limit int32) ([]*types.JobRow, error) {
	return nil, nil
}

func (r *repository) Ping() error {
	return nil
}
