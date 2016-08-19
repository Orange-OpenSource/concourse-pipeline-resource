package buildserver

import (
	"net/http"
	"strconv"

	"github.com/pivotal-golang/lager"
)

func (s *Server) AbortBuild(w http.ResponseWriter, r *http.Request) {
	buildID, err := strconv.Atoi(r.FormValue(":build_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	aLog := s.logger.Session("abort", lager.Data{
		"build": buildID,
	})

	build, found, err := s.db.GetBuild(buildID)
	if err != nil {
		aLog.Error("failed-to-get-build", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	engineBuild, err := s.engine.LookupBuild(aLog, build)
	if err != nil {
		aLog.Error("failed-to-lookup-build", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = engineBuild.Abort(aLog)
	if err != nil {
		aLog.Error("failed-to-abort-build", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
