package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"go.6river.tech/gosix-example/defaults"
	"go.6river.tech/gosix-example/ent"
	"go.6river.tech/gosix-example/ent/counter"
	"go.6river.tech/gosix-example/oas"
	"go.6river.tech/gosix/db/postgres"
	"go.6river.tech/gosix/logging"
	oastypes "go.6river.tech/gosix/oas"
	"go.6river.tech/gosix/server"
	"go.6river.tech/gosix/testutils"
)

func checkMainError(t *testing.T, err error) {
	t.Helper()
	// main died, errgroup will have called skip or fatal
	// if db doesn't exist, count this as a skip instead of a fail
	if _, ok := postgres.IsPostgreSQLErrorCode(err, postgres.InvalidCatalogName); ok {
		// we can't call skip from here, we need to check this again higher up
		t.Skip("Acceptance test DB does not exist, skipping test")
	}
	require.NoError(t, err, "main() should not panic")
}

func TestEndpoints(t *testing.T) {
	logging.ConfigureDefaultLogging()
	server.EnableRandomPorts()

	// setup server
	oldEnv := os.Getenv("NODE_ENV")
	defer os.Setenv("NODE_ENV", oldEnv)
	// this will target a postgresql db by default
	os.Setenv("NODE_ENV", "acceptance")
	eg, ctx := errgroup.WithContext(testutils.ContextForTest(t))
	app := NewApp()
	eg.Go(app.Main)

	client := http.DefaultClient
	var baseUrl = "http://localhost:" + strconv.Itoa(server.ResolvePort(defaults.Port, 0))

	// wait for app to start
	for {
		delay := time.After(time.Millisecond)
		select {
		case <-ctx.Done():
			checkMainError(t, eg.Wait())
			return
		case <-delay:
			// continue with checks...
		}

		if !app.Registry.ServicesStarted() {
			continue
		}
		if err := app.Registry.WaitAllReady(ctx); err != nil {
			checkMainError(t, eg.Wait())
		}
		break
	}

	// reset the app DB, except the default "frob" counter
	{
		ec, ok := app.EntClient()
		require.True(t, ok)
		_, err := ec.(*ent.Client).Counter.Delete().
			Where(counter.NameNEQ("frob")).
			Exec(ctx)
		require.NoError(t, err)
	}

	// load the OAS spec
	swagger := oas.MustLoadSpec()
	counterSchema := swagger.Components.Schemas["Counter"].Value

	assertCounterResult := func(t *testing.T, resp *http.Response, name string) gin.H {
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Contains(t, resp.Header.Get("Content-type"), "application/json")
		// want to assert on resp.ContentLength, but gzip encoding means we
		// don't have it, because it'd be the length of the compressed content,
		// but we get a gzip reader to see the uncompressed content
		bodyObject := map[string]interface{}{}
		err := json.NewDecoder(resp.Body).Decode(&bodyObject)
		assert.NoError(t, err)

		assert.Contains(t, bodyObject, "name")
		assert.Equal(t, name, bodyObject["name"])

		assert.Contains(t, bodyObject, "value")
		require.IsType(t, float64(0), bodyObject["value"])
		value := bodyObject["value"].(float64)
		assert.GreaterOrEqual(t, value, float64(0))

		assert.Contains(t, bodyObject, "id")
		assert.IsType(t, "", bodyObject["id"])
		id, err := uuid.Parse(bodyObject["id"].(string))
		assert.NoError(t, err)
		assert.NotZero(t, id)

		// TODO: use the full response validation for these, instead of just a hard
		// coded schema validation
		assert.NoError(t, counterSchema.VisitJSON(bodyObject), "Response body should validate against OAS Counter schema")
		return bodyObject
	}

	// use uuid to generate a unique string so our create calls cannot collide
	unique := uuid.New().String()

	tests := []struct {
		name   string
		url    string
		method string
		body   json.RawMessage
		check  func(*testing.T, *http.Response)
	}{
		{
			"uptime",
			"/",
			http.MethodGet,
			nil,
			func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Contains(t, resp.Header.Get("Content-type"), "application/json")
				bodyObject := map[string]interface{}{}
				err := json.NewDecoder(resp.Body).Decode(&bodyObject)
				assert.NoError(t, err)
				assert.Contains(t, bodyObject, "startTime")
				assert.IsType(t, "", bodyObject["startTime"])
				startTime, err := time.Parse(oastypes.OASRFC3339Millis, bodyObject["startTime"].(string))
				assert.NoError(t, err)
				assert.NotZero(t, startTime)
			},
		},

		{
			"GET frob",
			"/v1/counter/frob",
			http.MethodGet,
			nil,
			func(t *testing.T, resp *http.Response) {
				assertCounterResult(t, resp, "frob")
			},
		},
		{
			"POST(CREATE) frob-new",
			"/v1/counter/frob-" + unique,
			http.MethodPost,
			nil,
			func(t *testing.T, resp *http.Response) {
				assertCounterResult(t, resp, "frob-"+unique)
			},
		},

		{
			"POST(UPSERT-CREATE) frob-new2",
			"/v1/counter",
			http.MethodPost,
			rawJson(gin.H{"id": unique, "name": "frob-new-" + unique, "value": 0}),
			func(t *testing.T, resp *http.Response) {
				bodyObject := assertCounterResult(t, resp, "frob-new-"+unique)
				assert.Equal(t, float64(0), bodyObject["value"])
			},
		},
		{
			// this test assumes the prior test has run first
			"POST(UPSERT-UPDATE) frob-new2",
			"/v1/counter",
			http.MethodPost,
			rawJson(gin.H{"id": unique, "name": "frob-new-" + unique, "value": 2}),
			func(t *testing.T, resp *http.Response) {
				bodyObject := assertCounterResult(t, resp, "frob-new-"+unique)
				assert.Equal(t, float64(2), bodyObject["value"])
			},
		},
		{
			"POST(UPSERT-FAIL) frob-new2",
			"/v1/counter",
			http.MethodPost,
			rawJson(gin.H{"id": uuid.New(), "name": "frob-new-" + unique, "value": 2}),
			func(t *testing.T, resp *http.Response) {
				assert.Equal(t, http.StatusConflict, resp.StatusCode)
				assert.Contains(t, resp.Header.Get("Content-type"), "application/json")
				// see assertCounterResult for note about Content-length check
				bodyObject := map[string]interface{}{}
				err := json.NewDecoder(resp.Body).Decode(&bodyObject)
				assert.Contains(t, bodyObject, "name")
				assert.Equal(t, bodyObject["name"], "frob-new-"+unique)
				assert.Contains(t, bodyObject, "message")
				assert.Contains(t, bodyObject["message"], "exists")
				assert.NoError(t, err)
			},
		},

		{
			"shutdown",
			"/server/shutdown",
			http.MethodPost,
			nil,
			nil,
		},
	}

	// run tests, last one will close app
	for _, tt := range tests {
		if tt.name == "" {
			tt.name = tt.method + " " + tt.url
		}
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != nil {
				bodyReader = bytes.NewReader(tt.body)
			}
			// ctx is from the errgroup for running main(), so if that blows up the
			// request (and thus tests) get canceled instead of hanging until the
			// timeout.
			req, err := http.NewRequestWithContext(ctx, tt.method, baseUrl+tt.url, bodyReader)
			require.NoError(t, err)
			if bodyReader != nil {
				req.Header.Add("Content-Type", "application/json")
			}
			resp, err := client.Do(req)
			if resp != nil {
				defer resp.Body.Close()
			}
			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, resp)
			}
		})
	}

	app.Registry.RequestStopServices()
	err := eg.Wait()
	assert.NoError(t, err, "main should not panic")
}

func rawJson(data gin.H) json.RawMessage {
	msg, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return msg
}
