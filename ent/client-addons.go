package ent

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"go.6river.tech/gosix-example/ent/util"
	entcommon "go.6river.tech/gosix/ent"
)

// custom add-ons to the Client type for use in our environment

// BeginTxGeneric implements ginmiddleware.EntClient
func (c *Client) BeginTxGeneric(ctx context.Context, opts *sql.TxOptions) (entcommon.EntTx, error) {
	return c.BeginTx(ctx, opts)
}

func (c *Client) EntityClient(name string) entcommon.EntityClient {
	switch name {
	case "Counter":
		return c.Counter
	case "CounterEvent":
		return c.CounterEvent
	default:
		panic(errors.Errorf("Invalid entity name '%s'", name))
	}
}

func (c *Client) GetSchema() entcommon.EntClientSchema {
	return c.Schema
}

func (c *CounterClient) CreateEntity() entcommon.EntityCreate {
	return c.Create()
}

func (c *CounterEventClient) CreateEntity() entcommon.EntityCreate {
	return c.Create()
}

func (cc *CounterCreate) EntityMutation() ent.Mutation {
	return cc.Mutation()
}

func (cc *CounterEventCreate) EntityMutation() ent.Mutation {
	return cc.Mutation()
}

func (cc *CounterCreate) SaveEntity(ctx context.Context) (interface{}, error) {
	return cc.Save(ctx)
}

func (cc *CounterEventCreate) SaveEntity(ctx context.Context) (interface{}, error) {
	return cc.Save(ctx)
}

// DoTx wraps inner in a transaction, which will be committed if it returns nil
// or rolled back if it returns an error
func (c *Client) DoTx(ctx context.Context, opts *sql.TxOptions, inner func(*Tx) error) (finalErr error) {
	tx, finalErr := c.BeginTx(ctx, opts)
	if finalErr != nil {
		return
	}
	success := false
	defer func() {
		var err error
		var op string
		if !success {
			err = tx.Rollback()
			op = "Rollback"
		} else {
			err = tx.Commit()
			op = "Commit"
		}
		if err != nil {
			if finalErr != nil {
				finalErr = errors.Wrapf(finalErr, "%s Failed: %s During: %s", op, err.Error(), finalErr.Error())
			} else {
				finalErr = err
			}
		}
	}()

	finalErr = inner(tx)
	if finalErr == nil {
		success = true
	}
	return
}

func (cec *CounterEventClient) EventForCounter(c *Counter) *CounterEventCreate {
	return cec.EventForCounterId(c.ID)
}

func (cec *CounterEventClient) EventForCounterId(id uuid.UUID) *CounterEventCreate {
	m := cec.Create()
	util.EventForCounterId(m.Mutation(), id)
	return m
}
