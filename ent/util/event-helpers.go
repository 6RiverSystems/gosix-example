package util

import (
	"encoding/json"

	"github.com/google/uuid"

	"go.6river.tech/gosix/ent/mixins"
)

func EventForCounterId(m mixins.EventMutation, id uuid.UUID) {
	m.SetScopeType("counter")
	m.SetScopeId(id.String())
	m.SetData(json.RawMessage("{}"))
}
