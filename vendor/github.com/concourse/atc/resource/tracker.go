package resource

import (
	"github.com/concourse/atc"
	"github.com/concourse/atc/worker"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
)

type ResourceType string
type ContainerImage string

type Session struct {
	ID        worker.Identifier
	Metadata  worker.Metadata
	Ephemeral bool
}

//go:generate counterfeiter . Tracker

type Tracker interface {
	Init(lager.Logger, Metadata, Session, ResourceType, atc.Tags, atc.ResourceTypes, worker.ImageFetchingDelegate) (Resource, error)
	InitWithCache(lager.Logger, Metadata, Session, ResourceType, atc.Tags, CacheIdentifier, atc.ResourceTypes, worker.ImageFetchingDelegate) (Resource, Cache, error)
	InitWithSources(lager.Logger, Metadata, Session, ResourceType, atc.Tags, map[string]ArtifactSource, atc.ResourceTypes, worker.ImageFetchingDelegate) (Resource, []string, error)
}

//go:generate counterfeiter . Cache

type Cache interface {
	IsInitialized() (bool, error)
	Initialize() error
}

type Metadata interface {
	Env() []string
}

type tracker struct {
	workerClient worker.Client
	clock        clock.Clock
}

type TrackerFactory struct{}

func (factory TrackerFactory) TrackerFor(client worker.Client) Tracker {
	return NewTracker(client)
}

func NewTracker(workerClient worker.Client) Tracker {
	return &tracker{
		workerClient: workerClient,
		clock:        clock.NewClock(),
	}
}

type VolumeMount struct {
	Volume    worker.Volume
	MountPath string
}

func (tracker *tracker) InitWithSources(
	logger lager.Logger,
	metadata Metadata,
	session Session,
	typ ResourceType,
	tags atc.Tags,
	sources map[string]ArtifactSource,
	resourceTypes atc.ResourceTypes,
	imageFetchingDelegate worker.ImageFetchingDelegate,
) (Resource, []string, error) {
	logger = logger.Session("init-with-sources")

	logger.Debug("start")
	defer logger.Debug("done")

	container, found, err := tracker.workerClient.FindContainerForIdentifier(logger, session.ID)
	if err != nil {
		logger.Error("failed-to-look-for-existing-container", err)
		return nil, nil, err
	}

	if found {
		logger.Debug("found-existing-container", lager.Data{"container": container.Handle()})

		missingNames := []string{}

		for name, _ := range sources {
			missingNames = append(missingNames, name)
		}

		return NewResource(container, tracker.clock), missingNames, nil
	}

	resourceSpec := worker.ContainerSpec{
		ImageSpec: worker.ImageSpec{
			ResourceType: string(typ),
			Privileged:   true,
		},
		Ephemeral: session.Ephemeral,
		Tags:      tags,
		Env:       metadata.Env(),
	}

	compatibleWorkers, err := tracker.workerClient.AllSatisfying(resourceSpec.WorkerSpec(), resourceTypes)
	if err != nil {
		return nil, nil, err
	}

	// find the worker with the most volumes
	mounts := []worker.VolumeMount{}
	missingSources := []string{}
	var chosenWorker worker.Worker

	for _, w := range compatibleWorkers {
		candidateMounts := []worker.VolumeMount{}
		missing := []string{}

		for name, source := range sources {
			ourVolume, found, err := source.VolumeOn(w)
			if err != nil {
				return nil, nil, err
			}

			if found {
				candidateMounts = append(candidateMounts, worker.VolumeMount{
					Volume:    ourVolume,
					MountPath: ResourcesDir("put/" + name),
				})
			} else {
				missing = append(missing, name)
			}
		}

		if len(candidateMounts) >= len(mounts) {
			for _, mount := range mounts {
				mount.Volume.Release(nil)
			}

			mounts = candidateMounts
			missingSources = missing
			chosenWorker = w
		} else {
			for _, mount := range candidateMounts {
				mount.Volume.Release(nil)
			}
		}
	}

	resourceSpec.Inputs = mounts

	container, err = chosenWorker.CreateContainer(
		logger,
		nil,
		imageFetchingDelegate,
		session.ID,
		session.Metadata,
		resourceSpec,
		resourceTypes,
	)
	if err != nil {
		logger.Error("failed-to-create-container", err)
		return nil, nil, err
	}

	logger.Info("created", lager.Data{"container": container.Handle()})

	for _, mount := range mounts {
		mount.Volume.Release(nil)
	}

	return NewResource(container, tracker.clock), missingSources, nil
}

func (tracker *tracker) Init(
	logger lager.Logger,
	metadata Metadata,
	session Session,
	typ ResourceType,
	tags atc.Tags,
	resourceTypes atc.ResourceTypes,
	imageFetchingDelegate worker.ImageFetchingDelegate,
) (Resource, error) {
	logger = logger.Session("init")

	logger.Debug("start")
	defer logger.Debug("done")

	container, found, err := tracker.workerClient.FindContainerForIdentifier(logger, session.ID)
	if err != nil {
		logger.Error("failed-to-look-for-existing-container", err)
		return nil, err
	}

	if found {
		logger.Debug("found-existing-container", lager.Data{"container": container.Handle()})
		return NewResource(container, tracker.clock), nil
	}

	logger.Debug("creating-container")

	container, err = tracker.workerClient.CreateContainer(
		logger,
		nil,
		imageFetchingDelegate,
		session.ID,
		session.Metadata,
		worker.ContainerSpec{
			ImageSpec: worker.ImageSpec{
				ResourceType: string(typ),
				Privileged:   true,
			},
			Ephemeral: session.Ephemeral,
			Tags:      tags,
			Env:       metadata.Env(),
		},
		resourceTypes,
	)
	if err != nil {
		return nil, err
	}

	logger.Info("created", lager.Data{"container": container.Handle()})

	return NewResource(container, tracker.clock), nil
}

func (tracker *tracker) InitWithCache(
	logger lager.Logger,
	metadata Metadata,
	session Session,
	typ ResourceType,
	tags atc.Tags,
	cacheIdentifier CacheIdentifier,
	resourceTypes atc.ResourceTypes,
	imageFetchingDelegate worker.ImageFetchingDelegate,
) (Resource, Cache, error) {
	logger = logger.Session("init-with-cache")

	logger.Debug("start")
	defer logger.Debug("done")

	container, found, err := tracker.workerClient.FindContainerForIdentifier(logger, session.ID)
	if err != nil {
		logger.Error("failed-to-look-for-existing-container", err)
		return nil, nil, err
	}

	if found {
		logger.Debug("found-existing-container", lager.Data{"container": container.Handle()})

		resource := NewResource(container, tracker.clock)

		var cache Cache
		cacheVolume, found := resource.CacheVolume()
		if found {
			logger.Debug("found-cache")
			cache = volumeCache{cacheVolume}
		} else {
			logger.Debug("no-cache")
			cache = noopCache{}
		}

		return resource, cache, nil
	}

	logger.Debug("no-existing-container")

	resourceSpec := worker.WorkerSpec{
		ResourceType: string(typ),
		Tags:         tags,
	}

	chosenWorker, err := tracker.workerClient.Satisfying(resourceSpec, resourceTypes)
	if err != nil {
		logger.Info("no-workers-satisfying-spec", lager.Data{
			"error": err.Error(),
		})
		return nil, nil, err
	}

	containerSpec := worker.ContainerSpec{
		ImageSpec: worker.ImageSpec{
			ResourceType: string(typ),
			Privileged:   true,
		},
		Ephemeral: session.Ephemeral,
		Tags:      tags,
		Env:       metadata.Env(),
	}

	cachedVolume, cacheFound, err := cacheIdentifier.FindOn(logger, chosenWorker)
	if err != nil {
		logger.Error("failed-to-look-for-cache", err)
		return nil, nil, err
	}

	if cacheFound {
		logger.Debug("found-cache", lager.Data{"volume": cachedVolume.Handle()})
	} else {
		logger.Debug("no-cache-found")

		cachedVolume, err = cacheIdentifier.CreateOn(logger, chosenWorker)
		if err == worker.ErrNoVolumeManager {
			logger.Info("worker-has-no-volume-manager")
		} else if err != nil {
			logger.Error("failed-to-create-cache", err)
			return nil, nil, err
		}
	}

	if cachedVolume == nil {
		logger.Debug("creating-container-without-cache")
	} else {
		logger.Debug("creating-container-with-cache", lager.Data{
			"cache-handle": cachedVolume.Handle(),
		})

		defer cachedVolume.Release(nil)

		containerSpec.Outputs = []worker.VolumeMount{
			{
				Volume:    cachedVolume,
				MountPath: ResourcesDir("get"),
			},
		}
	}

	container, err = chosenWorker.CreateContainer(
		logger,
		nil,
		imageFetchingDelegate,
		session.ID,
		session.Metadata,
		containerSpec,
		resourceTypes,
	)
	if err != nil {
		logger.Error("failed-to-create-container", err)
		return nil, nil, err
	}

	logger.Info("created", lager.Data{"container": container.Handle()})

	return NewResource(container, tracker.clock), volumeCache{cachedVolume}, nil
}
