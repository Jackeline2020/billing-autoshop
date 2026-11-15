// Package config lê a configuração do serviço a partir de variáveis de
// ambiente.
package config

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Port devolve a porta HTTP do serviço (padrão 8080).
func Port() string {
	return Env("PORT", "8080")
}

// Env lê uma variável de ambiente, com valor padrão.
func Env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// PostgresDSN monta a conexão a partir de DB_HOST, DB_PORT, DB_USER,
// DB_PASSWORD, DB_NAME e DB_SSLMODE.
func PostgresDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		Env("DB_USER", "billing"),
		Env("DB_PASSWORD", "billing"),
		Env("DB_HOST", "localhost"),
		Env("DB_PORT", "5432"),
		Env("DB_NAME", "billing"),
		Env("DB_SSLMODE", "disable"),
	)
}

// NewPostgresPool cria o pool de conexões e confirma que o banco responde.
func NewPostgresPool(ctx context.Context) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, PostgresDSN())
	if err != nil {
		return nil, fmt.Errorf("erro ao criar pool do Postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("erro ao conectar ao Postgres: %w", err)
	}
	return pool, nil
}

// LoadAWSConfig carrega credenciais e região pela cadeia padrão do SDK
// (variáveis de ambiente, IRSA no Kubernetes, perfil local). A variável
// AWS_ENDPOINT_URL, lida nativamente pelo SDK, aponta para o LocalStack em
// desenvolvimento e homologação.
func LoadAWSConfig(ctx context.Context) (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(Env("AWS_REGION", "us-east-1")))
}
