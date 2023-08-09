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
