package db

import (
	"database/sql/driver"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// registerCodecs registers custom pgx type codecs on a freshly-opened
// connection. Called from pool.go via pgxpool.Config.AfterConnect.
//
// Currently registers:
//   - NUMERIC ↔ shopspring.Decimal (billing amounts, LLM rate multipliers)
//
// UUID, JSONB, and array types use pgx's native codecs — no registration
// needed. JSONB is scanned as []byte; the repository layer casts to
// json.RawMessage at the boundary.
func registerCodecs(conn *pgx.Conn) error {
	// Wrap pgx's stock numeric codec so SCAN/DECODE accept both shopspring
	// Decimal and *shopspring.Decimal targets with no external dep.
	conn.TypeMap().RegisterType(&pgtype.Type{
		Name:  "numeric",
		OID:   pgtype.NumericOID,
		Codec: &shopspringNumericCodec{base: pgtype.NumericCodec{}},
	})
	return nil
}

// shopspringNumericCodec delegates wire-format encode/decode to pgx's native
// NumericCodec, then layers Go-side conversion to shopspring.Decimal.
// Kept in-house rather than taking a dep on jackc/pgx-shopspring-decimal:
// the surface is ~40 LoC and avoids a third-party upgrade surface.
type shopspringNumericCodec struct {
	base pgtype.NumericCodec
}

func (c *shopspringNumericCodec) FormatSupported(format int16) bool {
	return c.base.FormatSupported(format)
}

func (c *shopspringNumericCodec) PreferredFormat() int16 {
	return c.base.PreferredFormat()
}

func (c *shopspringNumericCodec) PlanEncode(m *pgtype.Map, oid uint32, format int16, value any) pgtype.EncodePlan {
	switch v := value.(type) {
	case decimal.Decimal:
		return encodeDecimal{base: c.base.PlanEncode(m, oid, format, numericFromDecimal(v))}
	case *decimal.Decimal:
		if v == nil {
			return nil
		}
		return encodeDecimal{base: c.base.PlanEncode(m, oid, format, numericFromDecimal(*v))}
	}
	return c.base.PlanEncode(m, oid, format, value)
}

func (c *shopspringNumericCodec) PlanScan(m *pgtype.Map, oid uint32, format int16, target any) pgtype.ScanPlan {
	switch target.(type) {
	case *decimal.Decimal:
		var n pgtype.Numeric
		basePlan := c.base.PlanScan(m, oid, format, &n)
		if basePlan == nil {
			return nil
		}
		return scanDecimal{numeric: &n, target: target.(*decimal.Decimal), base: basePlan}
	}
	return c.base.PlanScan(m, oid, format, target)
}

func (c *shopspringNumericCodec) DecodeDatabaseSQLValue(m *pgtype.Map, oid uint32, format int16, src []byte) (driver.Value, error) {
	return c.base.DecodeDatabaseSQLValue(m, oid, format, src)
}

func (c *shopspringNumericCodec) DecodeValue(m *pgtype.Map, oid uint32, format int16, src []byte) (any, error) {
	raw, err := c.base.DecodeValue(m, oid, format, src)
	if err != nil || raw == nil {
		return raw, err
	}
	n, ok := raw.(pgtype.Numeric)
	if !ok {
		return raw, nil
	}
	return decimalFromNumeric(n)
}

type encodeDecimal struct{ base pgtype.EncodePlan }

func (p encodeDecimal) Encode(value any, buf []byte) ([]byte, error) {
	switch v := value.(type) {
	case decimal.Decimal:
		return p.base.Encode(numericFromDecimal(v), buf)
	case *decimal.Decimal:
		if v == nil {
			return nil, nil
		}
		return p.base.Encode(numericFromDecimal(*v), buf)
	}
	return p.base.Encode(value, buf)
}

type scanDecimal struct {
	numeric *pgtype.Numeric
	target  *decimal.Decimal
	base    pgtype.ScanPlan
}

func (p scanDecimal) Scan(src []byte, _ any) error {
	if err := p.base.Scan(src, p.numeric); err != nil {
		return err
	}
	d, err := decimalFromNumeric(*p.numeric)
	if err != nil {
		return err
	}
	*p.target = d
	return nil
}

// numericFromDecimal converts a shopspring.Decimal to pgtype.Numeric.
// Preserves exponent and sign. NaN and ±Inf become the NULL-equivalent
// Numeric (Valid=false) — PostgreSQL NUMERIC has no NaN on the wire.
func numericFromDecimal(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{
		Int:   d.Coefficient(),
		Exp:   d.Exponent(),
		Valid: true,
	}
}

// decimalFromNumeric converts a pgtype.Numeric to shopspring.Decimal.
// Returns the zero value when the NUMERIC is SQL NULL. Infinity values
// (NaN, +Inf, -Inf) in PostgreSQL 14+ are reported as an error — billing
// math should never produce them; surfacing the anomaly beats silent coercion.
func decimalFromNumeric(n pgtype.Numeric) (decimal.Decimal, error) {
	if !n.Valid {
		return decimal.Decimal{}, nil
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite {
		return decimal.Decimal{}, fmt.Errorf("decimal: NUMERIC %v is not finite", n)
	}
	if n.Int == nil {
		return decimal.Decimal{}, nil
	}
	return decimal.NewFromBigInt(new(big.Int).Set(n.Int), n.Exp), nil
}
