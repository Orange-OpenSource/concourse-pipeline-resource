package db

import (
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter . Lease

type Lease interface {
	Break()
}

type lease struct {
	conn   Conn
	logger lager.Logger

	attemptSignFunc func(Tx) (sql.Result, error)
	heartbeatFunc   func(Tx) (sql.Result, error)
	breakFunc       func()

	breakChan chan struct{}
	running   *sync.WaitGroup
}

func (l *lease) AttemptSign(interval time.Duration) (bool, error) {
	tx, err := l.conn.Begin()
	if err != nil {
		return false, err
	}

	defer tx.Rollback()

	result, err := l.attemptSignFunc(tx)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	if rows == 0 {
		return false, nil
	}

	err = tx.Commit()
	if err != nil {
		return false, err
	}

	return true, nil
}

func (l *lease) KeepSigned(interval time.Duration) {
	l.breakChan = make(chan struct{})
	l.running = &sync.WaitGroup{}
	l.running.Add(1)

	go l.keepLeased(interval)
}

func (l *lease) Break() {
	close(l.breakChan)
	l.running.Wait()

	if l.breakFunc != nil {
		l.breakFunc()
	}
}

func (l *lease) extend() error {
	tx, err := l.conn.Begin()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	result, err := l.heartbeatFunc(tx)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return errors.New("lease not found")
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (l *lease) keepLeased(interval time.Duration) {
	defer l.running.Done()

	ticker := time.NewTicker(interval / 2)
	defer ticker.Stop()

dance:
	for {
		select {
		case <-ticker.C:
			err := l.extend()
			if err != nil {
				l.logger.Error("failed-to-renew-lease", err)
				break dance
			}

			l.logger.Debug("renewed-the-lease")
		case <-l.breakChan:
			break dance
		}
	}
}
