package storage

import "encoding/json"

// EncodeRecord is the common JSON representation used for durable records.
func EncodeRecord(value any) ([]byte, error) { return json.Marshal(value) }

// DecodeRecord decodes a durable record while retaining unknown fields for
// forward-compatible snapshots through the standard JSON behavior.
func DecodeRecord(data []byte, target any) error { return json.Unmarshal(data, target) }
