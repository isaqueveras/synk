package storage

import "github.com/isaqueveras/synk/types"

// Storage is an interface that defines methods for interacting with job storage.
// It provides a method to retrieve available jobs from a specified queue.
type Storage interface {
	// Ping checks the connection to the storage system.
	// It returns an error if the connection is not successful.
	Ping() error

	// GetJobAvailable retrieves a list of available jobs from the specified queue.
	// It takes the name of the queue and a limit on the number of jobs to retrieve.
	// It returns a slice of pointers to JobRow and an error if the operation fails.
	GetJobAvailable(queue string, limit int32) ([]*types.JobRow, error)

	Insert(queue, kind string, args []byte) error
}
