package ent

import "go.6river.tech/gosix/ent"

// Custom addons to the Mutation type

func (m *CounterMutation) EntClient() ent.EntClient {
	return m.Client()
}
