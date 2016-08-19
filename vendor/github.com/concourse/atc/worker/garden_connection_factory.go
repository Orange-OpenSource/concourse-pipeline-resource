package worker

import (
	"net/http"

	gconn "github.com/cloudfoundry-incubator/garden/client/connection"
	"github.com/cloudfoundry-incubator/garden/routes"
	"github.com/concourse/atc/worker/transport"
	"github.com/concourse/retryhttp"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/rata"
)

//go:generate counterfeiter . GardenConnectionFactory
type GardenConnectionFactory interface {
	BuildConnection() gconn.Connection
}

type gardenConnectionFactory struct {
	db          transport.TransportDB
	logger      lager.Logger
	workerName  string
	retryPolicy transport.RetryPolicy
}

func NewGardenConnectionFactory(
	db transport.TransportDB,
	logger lager.Logger,
	workerName string,
	retryPolicy transport.RetryPolicy,
) GardenConnectionFactory {
	return &gardenConnectionFactory{
		db:          db,
		logger:      logger,
		workerName:  workerName,
		retryPolicy: retryPolicy,
	}
}

func (gcf *gardenConnectionFactory) BuildConnection() gconn.Connection {
	httpClient := &http.Client{
		Transport: &retryhttp.RetryRoundTripper{
			Logger:       gcf.logger.Session("retryable-http-client"),
			Sleeper:      clock.NewClock(),
			RetryPolicy:  gcf.retryPolicy,
			RoundTripper: transport.NewRoundTripper(gcf.workerName, gcf.db, &http.Transport{DisableKeepAlives: true}),
		},
	}

	hijackableClient := &retryhttp.RetryHijackableClient{
		Logger:           gcf.logger.Session("retry-hijackable-client"),
		Sleeper:          clock.NewClock(),
		RetryPolicy:      gcf.retryPolicy,
		HijackableClient: transport.NewHijackableClient(gcf.workerName, gcf.db, retryhttp.DefaultHijackableClient),
	}

	// the request generator's address doesn't matter because it's overwritten by the worker lookup clients
	hijackStreamer := &transport.WorkerHijackStreamer{
		HttpClient:       httpClient,
		HijackableClient: hijackableClient,
		Req:              rata.NewRequestGenerator("http://127.0.0.1:8080", routes.Routes),
	}

	return gconn.NewWithHijacker(hijackStreamer, gcf.logger)
}
