package schema

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"go.6river.tech/gosix-example/ent/util"
	entcommon "go.6river.tech/gosix/ent"
	"go.6river.tech/gosix/ent/mixins"
)

type Counter struct {
	ent.Schema
}

func (Counter) Fields() []ent.Field {
	return []ent.Field{
		// id is implicitly the primary key
		// have to assign a default up here because:
		// a) sqlite
		// b) ent bug: https://github.com/ent/ent/issues/781
		field.UUID("id", uuid.UUID{}).StorageKey("id").Default(uuid.New).Immutable(),
		field.String("name").Unique(),
		// defaulting this to zero makes it be omitted from JSON output
		field.Int64("value").Default(1),
	}
}

func (Counter) Annotations() []schema.Annotation {
	// TODO: figure out how to support schema qualification in ent
	return []schema.Annotation{
		entsql.Annotation{Table: "counter"},
	}
}

func (Counter) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("lastUpdate", CounterEvent.Type).
			Unique().
			StorageKey(edge.Column("last_update")).
			// TODO: this should be on delete restrict, but ent doesn't seem to allow
			// customization of that, and so it ends up as on delete cascade
			Required(),
	}
}

type counterMutationLike interface {
	ent.Mutation
	ID() (uuid.UUID, bool)
	SetLastUpdateID(uuid.UUID)

	EntClient() entcommon.EntClient
}

func (Counter) Hooks() []ent.Hook {
	return []ent.Hook{
		// auto-create an event
		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				// can't use the hook helpers here because they might not exist yet
				// skip this if we're doing a delete, or if the lastUpdate edge is already set
				if m.Op().Is(ent.OpDelete) || m.AddedIDs("lastUpdate") != nil {
					return next.Mutate(ctx, m)
				}
				cm := m.(counterMutationLike)
				id, hasId := cm.ID()
				if !hasId {
					return nil, errors.Errorf("Cannot auto-create event for counter %s without id", m.Op())
				}

				cec := cm.EntClient().EntityClient("CounterEvent").CreateEntity()
				evtMut := cec.EntityMutation().(mixins.EventMutation)
				util.EventForCounterId(evtMut, id)
				evtMut.SetEventType("auto:" + m.Op().String())
				_, err := cec.SaveEntity(ctx)
				if err != nil {
					return nil, err
				}
				// Save should fill in the ID on the mutation as part of applying
				// defaults
				evtId, ok := evtMut.ID()
				if !ok {
					return nil, errors.Errorf("event create mutation should have set ID")
				}
				cm.SetLastUpdateID(evtId)

				return next.Mutate(ctx, m)
			})
		},
	}
}

type CounterEvent struct {
	ent.Schema
}

func (CounterEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.EventStream{},
	}
}

func (CounterEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "counter_events"},
	}
}
