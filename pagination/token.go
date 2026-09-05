package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// tokenVersion stamps the encoding, so a token this build did not issue is
// rejected.
const tokenVersion = 1

// A cursor value is a JSON null where the column it names holds NULL, which no
// encoded value can collide with. page_size is deliberately not pinned, since
// AIP-158 requires a later call to be free to change it.
type token struct {
	Version int       `json:"v"`
	OrderBy string    `json:"by"`
	Scope   string    `json:"s"`
	Cursor  []*string `json:"c"`
}

func encodeToken[T any](order Order[T], scope string, last T) string {
	cursor := make([]*string, 0, len(order.columns))
	for _, col := range order.columns {
		cursor = append(cursor, col.Cursor(last))
	}

	// A token of strings and ints marshals without error.
	raw, _ := json.Marshal(token{
		Version: tokenVersion,
		OrderBy: order.canonical,
		Scope:   scope,
		Cursor:  cursor,
	})

	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeToken[T any](pageToken string, order Order[T], scope string) ([]any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(pageToken)
	if err != nil {
		return nil, errors.New("pagination: the page token is not base64url")
	}

	var decoded token
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, errors.New("pagination: the page token does not decode")
	}

	if decoded.Version != tokenVersion {
		return nil, errors.New("pagination: the page token was not issued by this build")
	}

	if decoded.OrderBy != order.canonical {
		return nil, errors.New("pagination: the page token was issued for another order_by")
	}

	if decoded.Scope != scope {
		return nil, errors.New("pagination: the page token was issued under different arguments")
	}

	if len(decoded.Cursor) != len(order.columns) {
		return nil, errors.New("pagination: the page token carries the wrong number of cursor values")
	}

	cursor := make([]any, 0, len(order.columns))

	for i, col := range order.columns {
		held := decoded.Cursor[i]

		if held == nil {
			if !col.Nullable() {
				return nil, fmt.Errorf(
					"pagination: the page token holds no value for %s, which is never NULL", col.Column)
			}

			cursor = append(cursor, nil)

			continue
		}

		value, err := col.ParseCursor(*held)
		if err != nil {
			return nil, fmt.Errorf("pagination: the page token carries an unreadable cursor value: %w", err)
		}

		cursor = append(cursor, value)
	}

	return cursor, nil
}
