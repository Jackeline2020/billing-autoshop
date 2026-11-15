package repository_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"billing-autoshop/pkg/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pool fica nil quando DB_HOST não está definida; nesse caso os testes de
// integração são ignorados.
var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	if os.Getenv("DB_HOST") != "" {
		var err error
		pool, err = config.NewPostgresPool(context.Background())
		if err != nil {
			panic(err)
		}
		resetSchema()
	}
	code := m.Run()
	if pool != nil {
		pool.Close()
	}
	os.Exit(code)
}

func requireDB(t *testing.T) {
	t.Helper()
	if pool == nil {
		t.Skip("DB_HOST não definida: teste de integração ignorado")
	}
}

// resetSchema recria o schema a partir das migrations versionadas, a mesma
// fonte usada nos ambientes.
func resetSchema() {
	ctx := context.Background()
	for _, stmt := range []string{`DROP SCHEMA public CASCADE`, `CREATE SCHEMA public`} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			panic(err)
		}
	}

	_, file, _, _ := runtime.Caller(0)
	paths, _ := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "*.up.sql"))
	sort.Strings(paths)
	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			panic(err)
		}
		for _, stmt := range strings.Split(string(content), ";") {
			if stmt = strings.TrimSpace(stmt); stmt != "" {
				if _, err := pool.Exec(ctx, stmt); err != nil {
					panic("erro na migration " + p + ": " + err.Error())
				}
			}
		}
	}
}
