package main

import (
	"context"
	"net/http"

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

	_ "github.com/jackc/pgx/v4"
	_ "github.com/mattn/go-sqlite3"

	_ "go.6river.tech/gosix-example/ent/runtime"
)

func main() {
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
		LoadOASSpec: func(context.Context) (*openapi3.Swagger, error) {
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
