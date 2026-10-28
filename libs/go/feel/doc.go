// Package feel adalah kernel murni berisi subset FEEL (Friendly Enough Expression Language,
// bahasa ekspresi Camunda 8) untuk kondisi sequence flow dan atribut zeebe:taskDefinition.
//
// Paket ini hanya memakai standard library. Alurnya:
//
//	src, isFEEL := feel.StripPrefix(raw)     // "=amount > 100" -> "amount > 100", true
//	expr, err := feel.Compile(src)           // *SyntaxError atau *UnsupportedError
//	ctx, err := feel.JSONContext(vars)       // map[string]json.RawMessage atau map[string][]byte
//	v, err := expr.Evaluate(ctx)
//	ok, isBool := v.Bool()
//
// Grammar subset (ADR-0013):
//
//	expr    = or
//	or      = and { "or" and }
//	and     = cmp { "and" cmp }
//	cmp     = unary [ ("=" | "!=" | "<" | "<=" | ">" | ">=") unary ]
//	unary   = "not" "(" expr ")" | primary
//	primary = number | string | "true" | "false" | "null" | path | "(" expr ")"
//	path    = name { "." name }
//
// Semantik konstruksi yang didukung identik dengan FEEL (Camunda 8.8 / feel-scala). Konstruksi FEEL
// di luar subset (aritmetika, pemanggilan fungsi, in, between, list/context literal, if, for,
// some/every, filter, instance of, temporal literal, komentar) ditolak saat Compile dengan
// *UnsupportedError, tidak pernah ditebak. Fase 7 memperluas subset tanpa mengubah API.
//
// Angka adalah desimal eksak (math/big.Rat), tidak pernah float64. Hasil null adalah nilai, bukan
// error. Expression dan Value immutable dan aman dipakai bersamaan oleh banyak goroutine.
package feel
