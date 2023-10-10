package testutils

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
)

func SetupDockerTest(t testing.TB) string {
	if testing.Short() {
		t.Skip("dockertest setup is not short, skipping test")
	}
	t.Log("using dockertest to create postgresql 14 db")
	pool, err := dockertest.NewPool("")
	require.NoError(t, err)
	require.NoError(t, pool.Client.PingWithContext(ContextForTest(t)))
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "14-alpine",
		Env: []string{
			"POSTGRES_USER=gosix",
			"POSTGRES_PASSWORD=gosix",
			"POSTGRES_DB=test",
		},
	}, func(hc *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err)
	t.Cleanup(func() { resource.Close() })
	hostAndPort := resource.GetHostPort("5432/tcp")
	dbUrl := fmt.Sprintf("postgres://gosix:gosix@%s/test?sslmode=disable", hostAndPort)
	// wait for the db server to be ready
	pool.MaxWait = 30 * time.Second
	require.NoError(t, pool.Retry(func() error {
		if db, err := sql.Open("pgx", dbUrl); err != nil {
			t.Log("DB not ready yet")
			return err
		} else {
			defer db.Close()
			return db.Ping()
		}
	}))
	t.Logf("DB is ready at %s", dbUrl)
	return dbUrl
}
