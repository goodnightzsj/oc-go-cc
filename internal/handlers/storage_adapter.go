package handlers

import (
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/storage"
)

type StorageWriter interface {
	InsertRequest(rec history.RequestRecord) error
}

type StorageAdapter struct {
	requests *storage.Requests
}

func NewStorageAdapter(db *storage.Database) *StorageAdapter {
	return &StorageAdapter{
		requests: storage.NewRequests(db),
	}
}

func (s *StorageAdapter) InsertRequest(rec history.RequestRecord) error {
	return s.requests.Insert(rec)
}
