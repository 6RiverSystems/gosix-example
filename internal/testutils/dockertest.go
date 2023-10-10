// Copyright (c) 2023 6 River Systems
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

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
