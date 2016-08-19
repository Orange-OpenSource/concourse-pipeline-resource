package workerserver

import (
	"time"

	"github.com/concourse/atc/db"
	"github.com/pivotal-golang/lager"
)

type Server struct {
	logger lager.Logger

	db WorkerDB
}

//go:generate counterfeiter . WorkerDB

type WorkerDB interface {
	SaveWorker(db.WorkerInfo, time.Duration) (db.SavedWorker, error)
	Workers() ([]db.SavedWorker, error)
}

func NewServer(
	logger lager.Logger,
	db WorkerDB,
) *Server {
	return &Server{
		logger: logger,
		db:     db,
	}
}
