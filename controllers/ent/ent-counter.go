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
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"go.6river.tech/gosix-example/ent"
	"go.6river.tech/gosix-example/ent/counter"
	"go.6river.tech/gosix-example/middleware"
	"go.6river.tech/gosix-example/oas"
	"go.6river.tech/gosix/db"
	"go.6river.tech/gosix/ginmiddleware"
	"go.6river.tech/gosix/logging"
	"go.6river.tech/gosix/registry"
)

// EntCounterController is a simple demo of how to make a controller, registered
// via a local `init` function, which uses `ent` and generated OAS types to
// serve requests.
type EntCounterController struct {
	logger *logging.Logger
}

const apiRoot = "/v1/counter"

func (cc *EntCounterController) Register(registry *registry.Registry, router gin.IRouter) error {
	if cc.logger == nil {
		cc.logger = logging.GetLogger("controllers/ent/counter")
	}

	rg := router.Group(apiRoot)
	rg.Use(ginmiddleware.WithTransaction(middleware.Key(), &sql.TxOptions{}))

	rg.GET("/:name", cc.GetCounter)
	rg.POST("/:name", cc.CreateCounter)
	rg.POST("/", cc.UpsertCounter)

	return nil
}

func (cc *EntCounterController) GetCounter(c *gin.Context) {
	name := c.Param("name")
	tx := middleware.Transaction(c)

	result, err := tx.Counter.Query().Where(counter.Name(name)).Only(c.Request.Context())
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, oas.ErrorMessage{Name: &name, Message: "Counter not found"})
			return
		}
		panic(err)
	}

	event := tx.CounterEvent.EventForCounter(result).
		SetEventType("auto-increment").
		SaveX(c.Request.Context())
	result = result.Update().
		AddValue(1).
		SetLastUpdate(event).
		SaveX(c.Request.Context())

	// This is a bit silly as the OAS and Ent types are so directly convertible,
	// but it demonstrates the pattern
	response := oas.Counter{
		Id:    result.ID,
		Name:  result.Name,
		Value: result.Value,
	}

	c.JSON(http.StatusOK, response)
}

func (cc *EntCounterController) CreateCounter(c *gin.Context) {
	name := c.Param("name")
	tx := middleware.Transaction(c)

	counterId := uuid.New()
	event := tx.CounterEvent.EventForCounterId(counterId).
		SetEventType("created").
		SaveX(c.Request.Context())
	result, err := tx.Counter.Create().
		SetName(name).
		SetLastUpdate(event).
		Save(c.Request.Context())
	if err != nil {
		cc.logger.Err(err).Str("name", name).Msg("failed to create counter")
		c.Error(err) //nolint:errcheck

		// two ways of checking this
		// checking for a real error from pgx here doesn't work, ent doesn't use error wrapping for this yet,
		// just string concatenation. Some day this may work however.
		var sqlError db.SQLError
		if errors.As(err, &sqlError) && sqlError.SQLState() == "23505" {
			c.JSON(http.StatusConflict, oas.ErrorMessage{Name: &name, Message: "Counter already exists"})
			return
		}
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, oas.ErrorMessage{Name: &name, Message: "Counter already exists"})
			return
		}

		panic(err)
	}

	cc.logger.Info().Interface("counter", result).Msg("created counter")

	// This is a bit silly as the OAS and Ent types are so directly convertible,
	// but it demonstrates the pattern
	response := oas.Counter{
		Id:    result.ID,
		Name:  result.Name,
		Value: result.Value,
	}

	c.JSON(http.StatusOK, response)
}

func (cc *EntCounterController) UpsertCounter(c *gin.Context) {
	// we rely on the validation middleware for most of the work ensuring the input object is OK

	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	bodyObject := &oas.Counter{}
	if err := decoder.Decode(&bodyObject); err != nil {
		panic(err)
	}
	id := bodyObject.Id

	tx := middleware.Transaction(c)
	event := tx.CounterEvent.EventForCounterId(id).
		SetEventType("upsert").
		SaveX(c.Request.Context())
	um := tx.Counter.UpdateOneID(id)
	mutationFromOAS(um.Mutation(), bodyObject)
	um.SetLastUpdate(event)
	counter, err := um.Save(c.Request.Context())
	if err != nil {
		if ent.IsNotFound(err) {
			// create the counter entity
			cm := tx.Counter.Create()
			mutationFromOAS(cm.Mutation(), bodyObject)
			cm.SetLastUpdate(event)
			counter, err = cm.Save(c.Request.Context())
		} else {
			panic(err)
		}
	}

	if err != nil {
		cc.logger.Err(err).Interface("request", bodyObject).Msg("failed to upsert counter")
		c.Error(err) //nolint:errcheck

		// TODO: share this with the create code

		// two ways of checking this
		// checking for a real error from pgx here doesn't work, ent doesn't use error wrapping for this yet,
		// just string concatenation. Some day this may work however.
		var sqlError db.SQLError
		if errors.As(err, &sqlError) && sqlError.SQLState() == "23505" {
			c.JSON(http.StatusConflict, oas.ErrorMessage{Name: &bodyObject.Name, Message: "Counter already exists"})
			return
		}
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, oas.ErrorMessage{Name: &bodyObject.Name, Message: "Counter already exists"})
			return
		}

		panic(err)
	}

	cc.logger.Info().Interface("counter", counter).Msg("upserted counter")

	// back door to test response validation
	if strings.Contains(counter.Name, "invalid") {
		c.JSON(http.StatusOK, gin.H{
			"id":    counter.ID.String(),
			"name":  counter.Name,
			"value": strconv.FormatInt(counter.Value, 10), // wrong type to show the validation error
		})
		return
	}

	// TODO: share this code
	// This is a bit silly as the OAS and Ent types are so directly convertible,
	// but it demonstrates the pattern
	response := oas.Counter{
		Id:    counter.ID,
		Name:  counter.Name,
		Value: counter.Value,
	}

	c.JSON(http.StatusOK, response)
}

func mutationFromOAS(mutation *ent.CounterMutation, dto *oas.Counter) {
	mutation.SetID(dto.Id)
	mutation.SetName(dto.Name)
	mutation.SetValue(dto.Value)
}
