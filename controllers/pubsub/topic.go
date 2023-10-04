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

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.6river.tech/gosix/logging"
	"go.6river.tech/gosix/pubsub"
	"go.6river.tech/gosix/registry"
)

// TODO: add the endpoints for this controller to the OAS spec

type TopicController struct {
	logger *logging.Logger
}

const apiRoot = "/v1/pubsub"

func (tc *TopicController) Register(reg *registry.Registry, router gin.IRouter) error {
	if tc.logger == nil {
		tc.logger = logging.GetLogger("controllers/pubsub/publisher")
	}

	reg.RegisterMap(router, apiRoot+"/topic", registry.HandlerMap{
		{http.MethodGet, ""}:                          tc.GetTopics,
		{http.MethodGet, "/"}:                         tc.GetTopics,
		{http.MethodGet, "/:id"}:                      tc.GetTopic,
		{http.MethodPost, "/:id"}:                     tc.CreateTopic,
		{http.MethodGet, "/:id/subscriptions"}:        tc.GetTopicSubscriptions,
		{http.MethodPost, "/:id/subscription/:subId"}: tc.CreateSubscription,
		{http.MethodPost, "/:id/publish"}:             tc.PublishMessage,
	})

	return nil
}

func (tc *TopicController) GetTopics(c *gin.Context) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	wroteHeader := false
	// demo streaming JSON info
	i := pubsub.MustDefaultClient().Topics(ctx)
	for {
		t, err := i.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// let gin try to report it, won't work if we already started writing results
			panic(fmt.Errorf("Error iterating topics: %w", err))
		}
		if !wroteHeader {
			c.Status(http.StatusOK)
			render.JSON{}.WriteContentType(c.Writer)
			// start a json array
			mustWriteString(c, "[\n")
			wroteHeader = true
		} else {
			// not the first one, write a delimiter
			mustWriteString(c, ",\n")
		}
		writeTopic(c, ctx, t, false)
		if err != nil {
			panic(fmt.Errorf("Error serializing topic info to JSON for '%s': %w", t.ID(), err))
		}
	}
	if wroteHeader {
		// terminate the JSON array
		mustWriteString(c, "\n]\n")
	}
}

func (tc *TopicController) GetTopic(c *gin.Context) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	id := c.Param("id")
	t := pubsub.MustDefaultClient().Topic(id)
	// TODO: this is wasteful: Exists will fetch the topic config under the hood,
	// and then the detail writer will do it again
	exists, err := t.Exists(ctx)
	if err != nil {
		panic(fmt.Errorf("Unable to check for topic existence: %w", err))
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"id": id})
	}
	writeTopic(c, ctx, t, true)
}

func (tc *TopicController) CreateTopic(c *gin.Context) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	id := c.Param("id")
	t, err := pubsub.MustDefaultClient().CreateTopic(ctx, id)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			c.JSON(http.StatusConflict, gin.H{"id": id, "message": "Topic already exists"})
			return
		}
		panic(fmt.Errorf("Failed to create topic '%s': %w", id, err))
	}

	writeTopic(c, ctx, t, true)
}

func writeTopic(c *gin.Context, ctx context.Context, t pubsub.Topic, writeHeader bool) {
	config, err := t.Config(ctx)
	if err != nil {
		panic(fmt.Errorf("Error fetching topic config for '%s': %w", t.ID(), err))
	}
	r := render.JSON{Data: gin.H{
		"id":     t.ID(),
		"config": config,
	}}
	if writeHeader {
		c.Status(http.StatusOK)
		r.WriteContentType(c.Writer)
	}
	if err = r.Render(c.Writer); err != nil {
		panic(fmt.Errorf("Error serializing topic info to JSON for '%s': %w", t.ID(), err))
	}
}

func (tc *TopicController) GetTopicSubscriptions(c *gin.Context) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	id := c.Param("id")
	t := pubsub.MustDefaultClient().Topic(id)
	if exists, err := t.Exists(ctx); err != nil {
		panic(fmt.Errorf("Unable to check for topic existence '%s': %w", id, err))
	} else if !exists {
		c.JSON(http.StatusNotFound, gin.H{"id": id, "message": "Topic not found"})
	}

	i := t.Subscriptions(ctx)
	writeSubscriptions(c, ctx, i)
}

func (tc *TopicController) CreateSubscription(c *gin.Context) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	topicID := c.Param("id")
	subID := c.Param("subId")
	client := pubsub.MustDefaultClient()
	t := client.Topic(topicID)
	s, err := t.CreateSubscription(ctx, subID, pubsub.SubscriptionConfig{
		// TODO: get config values from body object?
	})
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			c.JSON(http.StatusConflict, gin.H{"topic": topicID, "subscription": subID, "message": "Subscription already exists"})
			return
		}
		panic(fmt.Errorf("Failed to create subscription '%s'/'%s': %w", topicID, subID, err))
	}
	_, err = s.EnsureDefaultConfig(ctx)
	if err != nil {
		panic(fmt.Errorf("Failed to configure subscription '%s'/'%s': %w", topicID, subID, err))
	}

	writeSubscription(c, ctx, s, true)
}

func (tc *TopicController) PublishMessage(c *gin.Context) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	// validate the JSON but don't "parse" it
	var body json.RawMessage
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&body); err != nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
			"message": err.Error(),
			"details": err,
		})
	}

	id := c.Param("id")
	client := pubsub.MustDefaultClient()
	t := client.Topic(id)
	var exists bool
	if exists, err := t.Exists(ctx); err != nil {
		panic(fmt.Errorf("Unable to check for topic existence '%s': %w", id, err))
	} else if !exists {
		if t, err = client.CreateTopic(ctx, id); err != nil {
			panic(fmt.Errorf("Failed to create topic '%s': %w", id, err))
		}
	}

	pubId, err := t.Publish(ctx, &pubsub.RealMessage{
		Data: body,
		// TODO: allow sending Attributes via custom headers?
	}).Get(ctx)
	if err != nil {
		panic(fmt.Errorf("Failed to publish message to '%s': %w", id, err))
	}

	// we aren't going to re-use this object, so it's important to dispose of its
	// goroutines now
	t.Stop()

	// sent OK
	c.JSON(http.StatusCreated, gin.H{"id": pubId, "new": !exists})
	// CLI friendly
	mustWriteString(c, "\n")
}
