package db

import "encoding/json"

// DB-boundary helpers for NOT NULL collection / JSONB columns.
//
// Rule: a repository that writes to a NOT NULL text[] or JSONB column
// normalises nil/empty inputs through one of these helpers before handing
// the value to sqlc. Domain entities remain free to hold nil slices or
// empty json.RawMessage ("not provided"); the repository is the single
// boundary that materialises those as the empty collection Postgres
// requires.
//
// pgx v5 encodes a nil []string as SQL NULL (not '{}') and an empty
// json.RawMessage as SQL NULL (not '{}' / '[]'); Postgres DEFAULT clauses
// never fire for explicitly-supplied NULL, so the normalisation must
// happen on the Go side.

// NonNilStrings returns s if non-nil, else an empty slice.
// Use at write sites that target NOT NULL text[] columns.
func NonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// JSONOr returns raw if it contains any bytes, else fallback encoded as
// json.RawMessage. fallback must be a valid JSON literal such as "{}" or
// "[]" — it is the value the column takes when the caller did not supply
// one.
func JSONOr(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(fallback)
	}
	return raw
}
