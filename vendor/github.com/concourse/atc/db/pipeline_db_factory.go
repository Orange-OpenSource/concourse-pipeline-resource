package db

//go:generate counterfeiter . PipelineDBFactory

type PipelineDBFactory interface {
	Build(pipeline SavedPipeline) PipelineDB
	BuildWithID(pipelineID int) (PipelineDB, error)
	BuildWithTeamNameAndName(teamName, pipelineName string) (PipelineDB, error)
	BuildDefault() (PipelineDB, bool, error)
}

type pipelineDBFactory struct {
	conn        Conn
	bus         *notificationsBus
	pipelinesDB PipelinesDB
}

func NewPipelineDBFactory(
	sqldbConnection Conn,
	bus *notificationsBus,
	pipelinesDB PipelinesDB,
) *pipelineDBFactory {
	return &pipelineDBFactory{
		conn:        sqldbConnection,
		bus:         bus,
		pipelinesDB: pipelinesDB,
	}
}

func (pdbf *pipelineDBFactory) BuildWithID(pipelineID int) (PipelineDB, error) {
	savedPipeline, err := pdbf.pipelinesDB.GetPipelineByID(pipelineID)
	if err != nil {
		return nil, err
	}

	return pdbf.Build(savedPipeline), nil
}

func (pdbf *pipelineDBFactory) BuildWithTeamNameAndName(teamName, pipelineName string) (PipelineDB, error) {
	savedPipeline, err := pdbf.pipelinesDB.GetPipelineByTeamNameAndName(teamName, pipelineName)
	if err != nil {
		return nil, err
	}

	return pdbf.Build(savedPipeline), nil
}

func (pdbf *pipelineDBFactory) Build(pipeline SavedPipeline) PipelineDB {
	return &pipelineDB{
		conn: pdbf.conn,
		bus:  pdbf.bus,

		SavedPipeline: pipeline,
	}
}

func (pdbf *pipelineDBFactory) BuildDefault() (PipelineDB, bool, error) {
	orderedPipelines, err := pdbf.pipelinesDB.GetAllPipelines()
	if err != nil {
		return nil, false, err
	}

	if len(orderedPipelines) < 1 {
		return nil, false, nil
	}

	return &pipelineDB{
		conn: pdbf.conn,
		bus:  pdbf.bus,

		SavedPipeline: orderedPipelines[0],
	}, true, nil
}
