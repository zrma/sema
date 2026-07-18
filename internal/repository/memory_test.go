package repository_test

import (
	"testing"

	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/repository/repositorytest"
)

func TestMemoryRepositoryConformance(t *testing.T) {
	repositorytest.Run(t, func(t testing.TB) (repository.Repository, func() repository.Repository) {
		t.Helper()
		backend := repository.NewMemoryBackend()
		return repository.OpenMemory(backend), func() repository.Repository {
			return repository.OpenMemory(backend)
		}
	})
}

func BenchmarkMemoryRepository(b *testing.B) {
	repositorytest.BenchmarkAdapter(b, func(testing.TB) (repository.Repository, func() repository.Repository) {
		backend := repository.NewMemoryBackend()
		return repository.OpenMemory(backend), func() repository.Repository {
			return repository.OpenMemory(backend)
		}
	})
}
