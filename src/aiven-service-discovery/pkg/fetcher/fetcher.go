package fetcher

import (
	"sync"
	"time"

	"code.cloudfoundry.org/lager"
	aiven "github.com/aiven/aiven-go-client"
)

func init() {
	initMetrics()
}

const (
	defaultInterval = 120 * time.Second
	userAgent       = "govuk-paas-aiven-service-discovery-fetcher"
)

type Fetcher interface {
	Services() []aiven.Service

	Start()
	Stop()

	SetInterval(time.Duration)
}

type fetcher struct {
	aivenProject string
	aivenClient  aiven.Client

	logger lager.Logger

	stop chan struct{}
	wg   sync.WaitGroup

	interval time.Duration

	servicesMutex sync.RWMutex
	services      []aiven.Service
}

func NewFetcher(
	aivenProject string,
	aivenAPIToken string,

	logger lager.Logger,
) (Fetcher, error) {
	lsession := logger.Session("fetcher", lager.Data{"project": aivenProject})

	aivenClient, err := aiven.NewTokenClient(aivenAPIToken, userAgent)
	if err != nil {
		lsession.Error("err-aiven-new-token-client", err)
		return nil, err
	}

	f := fetcher{
		aivenProject: aivenProject,
		aivenClient:  *aivenClient,

		logger: lsession,

		stop: make(chan struct{}),

		interval: defaultInterval,
	}

	return &f, nil
}

func (f *fetcher) Services() []aiven.Service {
	f.servicesMutex.RLock()
	defer f.servicesMutex.RUnlock()

	return f.services
}

func (f *fetcher) fetch() {
	lsession := f.logger.Session("fetch")
	lsession.Info("begin")
	defer lsession.Info("end")

	FetcherFetchesTotal.Inc()

	aivenServices, err := f.aivenClient.Services.List(f.aivenProject)
	if err != nil {
		lsession.Error("err-aiven-services-list", err)
		FetcherAivenListServicesErrorsTotal.Inc()
		return
	}

	f.servicesMutex.Lock()
	defer f.servicesMutex.Unlock()

	services := make([]aiven.Service, 0)
	for _, service := range aivenServices {
		if service == nil {
			lsession.Info("skip-nil-service")
		} else {
			services = append(services, *service)
		}
	}

	f.services = services
}

func (f *fetcher) loop() {
	lsession := f.logger.Session("loop")
	lsession.Info("begin")
	defer lsession.Info("end")

	f.wg.Add(1)

	for {
		select {
		case <-time.After(f.interval):
			f.fetch()
		case <-f.stop:
			f.wg.Done()
			return
		}
	}
}

func (f *fetcher) Start() {
	lsession := f.logger.Session("start")
	lsession.Info("begin")
	defer lsession.Info("end")

	go f.loop()
}

func (f *fetcher) Stop() {
	lsession := f.logger.Session("stop")
	lsession.Info("begin")
	defer lsession.Info("end")

	close(f.stop)
	f.wg.Wait()
}

func (f *fetcher) SetInterval(interval time.Duration) {
	f.interval = interval
}
