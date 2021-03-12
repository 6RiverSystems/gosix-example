package controllers

import (
	"go.6river.tech/gosix-example/controllers/ent"
	"go.6river.tech/gosix-example/controllers/pubsub"
	"go.6river.tech/gosix/registry"
)

func RegisterAll(r *registry.Registry) {
	r.AddController(&ent.EntCounterController{})
	r.AddController(&pubsub.SubscriptionController{})
	r.AddController(&pubsub.TopicController{})
}
