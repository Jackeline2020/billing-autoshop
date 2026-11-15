package repository_test

import (
	"testing"

	"billing-autoshop/internal/repository"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessedMessageRepository(t *testing.T) {
	requireDB(t)
	repo := repository.NewProcessedMessagePostgresRepository(pool)
	id := uuid.NewString()

	exists, err := repo.Exists(id)
	require.NoError(t, err)
	assert.False(t, exists)

	require.NoError(t, repo.MarkProcessed(id))
	require.NoError(t, repo.MarkProcessed(id)) // repetir não é erro
	exists, err = repo.Exists(id)
	require.NoError(t, err)
	assert.True(t, exists)
}
