package service

import (
	"github.com/zrma/sema/internal/discovery"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

// CandidateIndex is a derived queue accelerator owned beside repository state.
// RepositoryVersion is a freshness fence, not an independent source of truth.
type CandidateIndex struct {
	repositoryVersion repository.Version
	index             discovery.Index
}

func BuildCandidateIndex(snapshot PlanningSnapshot) CandidateIndex {
	return CandidateIndex{
		repositoryVersion: snapshot.repositoryVersion,
		index:             discovery.BuildIndex(cloneTickets(snapshot.input.MatchTickets)),
	}
}

func (index CandidateIndex) RepositoryVersion() repository.Version {
	return index.repositoryVersion
}

// SelectWindow refuses to serve an index built from another repository version.
// An owner may rebuild or incrementally reconcile the index before retrying.
func (index CandidateIndex) SelectWindow(
	snapshot PlanningSnapshot,
	slots []int,
	limit int,
) (discovery.Window, error) {
	if snapshot.repositoryVersion != index.repositoryVersion {
		return discovery.Window{}, domain.NewFailure(
			domain.FailureStaleSnapshot,
			"candidate index is at repository version %d; snapshot requires %d",
			index.repositoryVersion,
			snapshot.repositoryVersion,
		)
	}
	window := index.index.SelectWindow(slots, limit)
	window.Tickets = cloneTickets(window.Tickets)
	return window, nil
}
