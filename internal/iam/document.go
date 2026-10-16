package iam

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Effect is a statement's effect: Allow or Deny.
type Effect string

const (
	// EffectAllow grants the statement's action on the resource.
	EffectAllow Effect = "Allow"
	// EffectDeny explicitly denies the action on the resource. An explicit
	// deny overrides any allow, anywhere in the applicable policies.
	EffectDeny Effect = "Deny"
)

// StringOrSlice accepts a JSON string or array of strings. AWS policy elements
// such as Action and Resource accept either form, so this type preserves that
// ergonomics. It always marshals back to an array for deterministic storage.
type StringOrSlice []string

// UnmarshalJSON implements json.Unmarshaler.
func (s *StringOrSlice) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = nil
		return nil
	}
	if b[0] == '"' {
		var one string
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*s = StringOrSlice{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

// MarshalJSON implements json.Marshaler.
func (s StringOrSlice) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}
	return json.Marshal([]string(s))
}

// Statement is a single policy statement.
//
// Exactly one of Action / NotAction and exactly one of Resource / NotResource
// must be present. Condition is modelled so the document shape is stable for
// v2, but validation rejects it in v1 (the evaluator cannot honour an
// un-evaluatable deny, and silently ignoring one would be a security hole).
type Statement struct {
	Sid         string                    `json:"Sid,omitempty"`
	Effect      Effect                    `json:"Effect"`
	Action      StringOrSlice             `json:"Action,omitempty"`
	NotAction   StringOrSlice             `json:"NotAction,omitempty"`
	Resource    StringOrSlice             `json:"Resource,omitempty"`
	NotResource StringOrSlice             `json:"NotResource,omitempty"`
	Condition   map[string]map[string]any `json:"Condition,omitempty"`
}

// StatementList accepts a JSON array of statements or a single statement
// object, matching AWS.
type StatementList []Statement

// UnmarshalJSON implements json.Unmarshaler.
func (l *StatementList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*l = nil
		return nil
	}
	if b[0] == '{' {
		var one Statement
		if err := strictUnmarshal(b, &one); err != nil {
			return err
		}
		*l = StatementList{one}
		return nil
	}
	var many []Statement
	if err := strictUnmarshal(b, &many); err != nil {
		return err
	}
	*l = many
	return nil
}

// PolicyDocument is a complete policy: a version and one or more statements.
type PolicyDocument struct {
	Version   string        `json:"Version,omitempty"`
	Statement StatementList `json:"Statement"`
}

// ParseDocument unmarshals a policy document. It performs structural decoding
// only; callers must run Validate before persisting or trusting the document.
// Unknown fields are rejected so typos ("Statment") fail loudly.
func ParseDocument(b []byte) (*PolicyDocument, error) {
	var doc PolicyDocument
	if err := strictUnmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("iam: invalid policy document: %w", err)
	}
	return &doc, nil
}

// Bytes marshals the document to canonical JSON.
func (doc *PolicyDocument) Bytes() ([]byte, error) {
	return json.Marshal(doc)
}

// strictUnmarshal decodes JSON into v, rejecting unknown fields.
func strictUnmarshal(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
