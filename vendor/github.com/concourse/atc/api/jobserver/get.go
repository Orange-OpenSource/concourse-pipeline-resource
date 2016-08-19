package jobserver

import (
	"encoding/json"
	"net/http"

	"github.com/concourse/atc/api/present"
	"github.com/concourse/atc/db"
)

func (s *Server) GetJob(pipelineDB db.PipelineDB) http.Handler {
	logger := s.logger.Session("get-job")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobName := r.FormValue(":job_name")

		config, _, found, err := pipelineDB.GetConfig()
		if err != nil {
			logger.Error("could-not-get-pipeline-config", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		job, found := config.Jobs.Lookup(jobName)
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		finished, next, err := pipelineDB.GetJobFinishedAndNextBuild(jobName)
		if err != nil {
			logger.Error("could-not-get-job-finished-and-next-build", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		dbJob, err := pipelineDB.GetJob(job.Name)
		if err != nil {
			logger.Error("could-not-get-job-finished", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)

		json.NewEncoder(w).Encode(present.Job(dbJob, job, config.Groups, finished, next))
	})
}
