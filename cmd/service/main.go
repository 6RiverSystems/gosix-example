// Copyright (c) 2021 6 River Systems
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

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"

	"entgo.io/ent/dialect/sql"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"go.6river.tech/gosix-example/controllers"
	_ "go.6river.tech/gosix-example/controllers"
	"go.6river.tech/gosix-example/defaults"
	"go.6river.tech/gosix-example/ent"
	"go.6river.tech/gosix-example/ent/counter"
	"go.6river.tech/gosix-example/migrations"
	"go.6river.tech/gosix-example/oas"
	"go.6river.tech/gosix-example/version"
	"go.6river.tech/gosix/app"
	_ "go.6river.tech/gosix/controllers"
	entcommon "go.6river.tech/gosix/ent"
	"go.6river.tech/gosix/ent/mixins"
	"go.6river.tech/gosix/logging"
	"go.6river.tech/gosix/migrate"
	"go.6river.tech/gosix/registry"

	_ "go.6river.tech/gosix-example/ent/runtime"
)

var testModeIgnoreArgs = false

func main() {
	if len(os.Args) > 1 {
		if len(os.Args) == 2 {
			switch os.Args[1] {
			case "--help":
				fmt.Println("gosix-example does not accept command line arguments")
				return
			case "--version":
				fmt.Printf("This is gosix-example version %s running on %s/%s\n",
					version.SemrelVersion, runtime.GOOS, runtime.GOARCH)
				return
			}
		}

		if !testModeIgnoreArgs {
			fmt.Fprintf(os.Stderr, "gosix-example does not accept command line arguments: %v\n", os.Args[1:])
			os.Exit(1)
		}
	}

	if err := NewApp().Main(); err != nil {
		panic(err)
	}
}

func NewApp() *app.App {
	const appName = "gosix-example"
	app := &app.App{
		Name:    appName,
		Version: version.SemrelVersion,
		Port:    defaults.Port,
		InitDbMigration: func(ctx context.Context, m *migrate.Migrator) error {
			// merge our own migrations with the event stream ones
			// event stream goes first so our local migrations can reference it
			m.SortAndAppend("counter_events/", mixins.EventMigrationsFor("public", "counter_events")...)
			ownMigrations, err := migrate.LoadFS(migrations.CounterMigrations, nil)
			if err != nil {
				return err
			}
			m.SortAndAppend("", ownMigrations...)
			return nil
		},
		InitEnt: func(ctx context.Context, drv *sql.Driver, logger func(args ...interface{}), debug bool) (entcommon.EntClient, error) {
			opts := []ent.Option{
				ent.Driver(drv),
				ent.Log(logger),
			}
			if debug {
				opts = append(opts, ent.Debug())
			}
			return ent.NewClient(opts...), nil
		},
		WithPubsubClient: true,
		LoadOASSpec: func(context.Context) (*openapi3.T, error) {
			return oas.LoadSpec()
		},
		OASFS: http.FS(oas.OpenAPIFS),
		CustomizeRoutes: func(_ context.Context, _ *gin.Engine, r *registry.Registry, _ registry.MutableValues) error {
			controllers.RegisterAll(r)
			return nil
		},
		RegisterServices: func(ctx context.Context, reg *registry.Registry, values registry.MutableValues) error {
			reg.AddService(registry.NewInitializer(
				"counter-bootstrap",
				func(ctx context.Context, services *registry.Registry, client_ entcommon.EntClient) error {
					client := client_.(*ent.Client)
					// create the default frob counter
					ec, err := getOrCreateCounterEnt(ctx, client, "frob")
					logger := logging.GetLogger(appName)
					if err != nil {
						logger.Err(err).Msg("Failed to init counter")
						return err
					}
					logger.Info().
						Interface("counter", ec).
						Msg("Ent Frob counter")
					return nil
				},
				nil,
			))
			return nil
		},
	}
	app.WithDefaults()
	return app
}

func getOrCreateCounterEnt(ctx context.Context, client *ent.Client, name string) (result *ent.Counter, err error) {
	err = client.DoTx(ctx, nil, func(tx *ent.Tx) (err error) {
		result, err = tx.Counter.Query().Where(counter.Name(name)).Only(ctx)
		if err == nil {
			// always test the hooks
			result, err = result.Update().AddValue(1).Save(ctx)
		} else if ent.IsNotFound(err) {
			counterId := uuid.New()
			// we could create the event explicitly here, but instead we rely on the
			// hook to do it for us as an example
			// event := client.CounterEvent.EventForCounterId(counterId).
			// 	SetEventType("created").
			// 	SaveX(ctx)
			result, err = tx.Counter.Create().
				SetID(counterId).
				SetName(name).
				// SetLastUpdate(event).
				Save(ctx)
		}
		// else return err as-is
		return
	})
	return
}
