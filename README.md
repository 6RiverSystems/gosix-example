# Go Six Example

An example for how to use [6RiverSystems/gosix](gosix) to build a simple
application

## Developing

Using `make`:

    make # does everything

There are other `make` targets to run individual steps as well

## Running

Using `go run`:

    go run ./cmd/server

Making a binary and running it

    make binaries
    ./bin/service

### Environment variables

How the app runs is controlled by several environment variables

- `NODE_ENV`
  - Set to one of `test`, `acceptance`, `development`, or `production`
  - Defaults to `development`
  - `test` will default it to using SQLite instead of PostgreSQL
- `LOG_LEVEL`
  - Set to one of `trace`, `debug`, `info`, `warn`, `error`
  - Defaults to `info` in `production`, or `debug` elsewhere
- `DATABASE_URL`
  - Set to either `postgres://...` or `sqlite://...` to use a specific database

## Debugging

There are vscode launch configs for the app.
