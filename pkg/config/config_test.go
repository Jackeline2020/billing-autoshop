package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvAndPort(t *testing.T) {
	t.Setenv("PORT", "")
	assert.Equal(t, "8080", Port())
	t.Setenv("PORT", "9090")
	assert.Equal(t, "9090", Port())
}

func TestPostgresDSN(t *testing.T) {
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_NAME", "n")
	t.Setenv("DB_SSLMODE", "require")
	assert.Equal(t, "postgres://u:p@db:5433/n?sslmode=require", PostgresDSN())
}

func TestNewPostgresPool_Unreachable(t *testing.T) {
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "1")
	_, err := NewPostgresPool(context.Background())
	assert.Error(t, err)
}

func TestNewPostgresPool_InvalidDSN(t *testing.T) {
	t.Setenv("DB_PORT", "porta-invalida")
	_, err := NewPostgresPool(context.Background())
	assert.Error(t, err)
}
