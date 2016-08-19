package algorithm

import (
	"fmt"
	"strings"
)

type InputCandidates []InputVersionCandidates

type InputVersionCandidates struct {
	Input                 string
	Passed                JobSet
	UseEveryVersion       bool
	PinnedVersionID       int
	ExistingBuildResolver *ExistingBuildResolver
	usingEveryVersion     *bool

	VersionCandidates
}

func (inputVersionCandidates InputVersionCandidates) UsingEveryVersion() bool {
	if inputVersionCandidates.usingEveryVersion == nil {
		usingEveryVersion := inputVersionCandidates.UseEveryVersion &&
			inputVersionCandidates.ExistingBuildResolver.Exists()
		inputVersionCandidates.usingEveryVersion = &usingEveryVersion
	}

	return *inputVersionCandidates.usingEveryVersion
}

const VersionEvery = "every"

func (candidates InputCandidates) String() string {
	lens := []string{}
	for _, vcs := range candidates {
		lens = append(lens, fmt.Sprintf("%s (%d candidates for %d versions)", vcs.Input, len(vcs.VersionCandidates), len(vcs.VersionIDs())))
	}

	return fmt.Sprintf("[%s]", strings.Join(lens, "; "))
}

func (candidates InputCandidates) Reduce(jobs JobSet) (InputMapping, bool, MissingInputReasons) {
	return candidates.reduce(jobs, nil)
}

func (candidates InputCandidates) reduce(jobs JobSet, lastSatisfiedMapping InputMapping) (InputMapping, bool, MissingInputReasons) {
	newInputCandidates := candidates.pruneToCommonBuilds(jobs)

	for i, inputVersionCandidates := range newInputCandidates {
		versionIDs := inputVersionCandidates.VersionIDs()

		switch {
		case len(versionIDs) == 1:
			// already reduced
			continue
		case inputVersionCandidates.PinnedVersionID != 0:
			limitedToVersion := inputVersionCandidates.ForVersion(inputVersionCandidates.PinnedVersionID)

			inputCandidates := newInputCandidates[i]
			inputCandidates.VersionCandidates = limitedToVersion
			newInputCandidates[i] = inputCandidates
		default:
			usingEveryVersion := inputVersionCandidates.UsingEveryVersion()

			for j, id := range versionIDs {
				buildForPreviousOrCurrentVersionExists := func() bool {
					return inputVersionCandidates.ExistingBuildResolver.ExistsForVersion(id) ||
						j == len(versionIDs)-1 ||
						inputVersionCandidates.ExistingBuildResolver.ExistsForVersion(versionIDs[j+1])
				}

				limitedToVersion := inputVersionCandidates.ForVersion(id)

				inputCandidates := newInputCandidates[i]
				inputCandidates.VersionCandidates = limitedToVersion
				newInputCandidates[i] = inputCandidates

				// as we reduce we only care about final missing input reasons
				mapping, ok, _ := newInputCandidates.reduce(jobs, lastSatisfiedMapping)
				if ok {
					lastSatisfiedMapping = mapping
					if !usingEveryVersion || buildForPreviousOrCurrentVersionExists() {
						return mapping, true, MissingInputReasons{}
					}
				} else {
					if usingEveryVersion && (lastSatisfiedMapping != nil || buildForPreviousOrCurrentVersionExists()) {
						return lastSatisfiedMapping, true, MissingInputReasons{}
					}
				}

				newInputCandidates[i] = inputVersionCandidates
			}
		}
	}

	mapping := InputMapping{}
	missingInputReasons := MissingInputReasons{}

	for _, inputVersionCandidates := range newInputCandidates {
		versionIDs := inputVersionCandidates.VersionIDs()

		if len(versionIDs) != 1 || !inputVersionCandidates.JobIDs().Equal(inputVersionCandidates.Passed) {
			missingInputReasons.RegisterPassedConstraint(inputVersionCandidates.Input)
		} else {
			mapping[inputVersionCandidates.Input] = versionIDs[0]
		}
	}

	if len(missingInputReasons) > 0 {
		return nil, false, missingInputReasons
	}

	return mapping, true, missingInputReasons
}

func (candidates InputCandidates) pruneToCommonBuilds(jobs JobSet) InputCandidates {
	newCandidates := make(InputCandidates, len(candidates))
	copy(newCandidates, candidates)

	for jobID, _ := range jobs {
		commonBuildIDs := newCandidates.commonBuildIDs(jobID)

		for i, versionCandidates := range newCandidates {
			inputCandidates := versionCandidates
			inputCandidates.VersionCandidates = versionCandidates.PruneVersionsOfOtherBuildIDs(jobID, commonBuildIDs)
			newCandidates[i] = inputCandidates
		}
	}

	return newCandidates
}

func (candidates InputCandidates) commonBuildIDs(jobID int) BuildSet {
	firstTick := true

	var commonBuildIDs BuildSet

	for _, set := range candidates {
		setBuildIDs := set.BuildIDs(jobID)
		if len(setBuildIDs) == 0 {
			continue
		}

		if firstTick {
			commonBuildIDs = setBuildIDs
		} else {
			commonBuildIDs = commonBuildIDs.Intersect(setBuildIDs)
		}

		firstTick = false
	}

	return commonBuildIDs
}
