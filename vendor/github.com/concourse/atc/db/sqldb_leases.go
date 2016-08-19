package db

import (
	"database/sql"
	"time"

	"github.com/pivotal-golang/lager"
)

func (db *SQLDB) LeaseBuildTracking(logger lager.Logger, buildID int, interval time.Duration) (Lease, bool, error) {
	lease := &lease{
		conn: db.conn,
		logger: logger.Session("lease", lager.Data{
			"build_id": buildID,
		}),
		attemptSignFunc: func(tx Tx) (sql.Result, error) {
			return tx.Exec(`
				UPDATE builds
				SET last_tracked = now()
				WHERE id = $1
					AND now() - last_tracked > ($2 || ' SECONDS')::INTERVAL
			`, buildID, interval.Seconds())
		},
		heartbeatFunc: func(tx Tx) (sql.Result, error) {
			return tx.Exec(`
				UPDATE builds
				SET last_tracked = now()
				WHERE id = $1
			`, buildID)
		},
	}

	renewed, err := lease.AttemptSign(interval)
	if err != nil {
		return nil, false, err
	}

	if !renewed {
		return nil, renewed, nil
	}

	lease.KeepSigned(interval)

	return lease, true, nil
}

func (db *SQLDB) LeaseBuildScheduling(logger lager.Logger, buildID int, interval time.Duration) (Lease, bool, error) {
	lease := &lease{
		conn: db.conn,
		logger: logger.Session("lease", lager.Data{
			"build_id": buildID,
		}),
		attemptSignFunc: func(tx Tx) (sql.Result, error) {
			return tx.Exec(`
				UPDATE builds
				SET last_scheduled = now()
				WHERE id = $1
					AND now() - last_scheduled > ($2 || ' SECONDS')::INTERVAL
			`, buildID, interval.Seconds())
		},
		heartbeatFunc: func(tx Tx) (sql.Result, error) {
			return tx.Exec(`
				UPDATE builds
				SET last_scheduled = now()
				WHERE id = $1
			`, buildID)
		},
	}

	renewed, err := lease.AttemptSign(interval)
	if err != nil {
		return nil, false, err
	}

	if !renewed {
		return nil, renewed, nil
	}

	lease.KeepSigned(interval)

	return lease, true, nil
}

func (db *SQLDB) GetLease(logger lager.Logger, taskName string, interval time.Duration) (Lease, bool, error) {
	lease := &lease{
		conn: db.conn,
		logger: logger.Session("lease", lager.Data{
			"CacheInvalidator": "Scottsboro",
		}),
		attemptSignFunc: func(tx Tx) (sql.Result, error) {
			_, err := tx.Exec(`
				INSERT INTO leases (last_invalidated, name)
				SELECT 'epoch', $1
				WHERE NOT EXISTS (SELECT * FROM leases WHERE name = $1)`, taskName)
			if err != nil {
				return nil, err
			}
			return tx.Exec(`
				UPDATE leases
				SET last_invalidated = now()
				WHERE (now() - last_invalidated > ($1 || ' SECONDS')::INTERVAL) AND name = $2
			`, interval.Seconds(), taskName)
		},
		heartbeatFunc: func(tx Tx) (sql.Result, error) {
			return tx.Exec(`
				UPDATE leases
				SET last_invalidated = now()
				WHERE name = $1
			`, taskName)
		},
	}

	renewed, err := lease.AttemptSign(interval)
	if err != nil {
		return nil, false, err
	}

	if !renewed {
		return nil, renewed, nil
	}

	lease.KeepSigned(interval)

	return lease, true, nil
}
