# feel

Kernel murni (hanya standard library) berisi **subset FEEL** untuk kondisi sequence flow dan
`zeebe:taskDefinition` `type`/`retries` (ADR-0013, rencana Fase 2 bagian 3.4). Semantik konstruksi
yang didukung identik dengan FEEL Camunda 8.8 (feel-scala); konstruksi lain ditolak saat `Compile`
dengan `*UnsupportedError`, tidak pernah ditebak. Fase 7 memperluas subset tanpa mengubah API.

```go
src, isFEEL := feel.StripPrefix(`=order.total > 100`)
expr, err := feel.Compile(src)          // *SyntaxError | *UnsupportedError (dengan Pos byte offset)
ctx, err := feel.JSONContext(vars)      // map[string]json.RawMessage atau map[string][]byte
v, _ := expr.Evaluate(ctx)
ok, isBool := v.Bool()
```

## Subset yang didukung

```
expr    = or
or      = and { "or" and }
and     = cmp { "and" cmp }
cmp     = unary [ ("=" | "!=" | "<" | "<=" | ">" | ">=") unary ]
unary   = "not" "(" expr ")" | primary
primary = number | string | "true" | "false" | "null" | path | "(" expr ")"
path    = name { "." name }
```

- Angka: `42`, `1.5`, `.5`, `-5` (minus hanya pada literal). Desimal eksak (`math/big.Rat`), JSON
  dibaca dengan `UseNumber`, jadi `9007199254740993` dibandingkan persis.
- String: kutip ganda dengan escape `\"`, `\'`, `\\`, `\n`, `\r`, `\t`, `\uXXXX` (termasuk pasangan
  surrogate).
- Nama: huruf (Unicode), digit, `_`, `$`, tidak diawali digit, tanpa spasi.
- Ditolak sebagai `UnsupportedError`: aritmetika, pemanggilan fungsi selain `not(...)`, `in`,
  `between`, `instance of`, list/context literal, filter `[...]`, `if/then/else`, `some/every`, `for`,
  temporal literal `@"..."`, nama backtick, komentar, perbandingan berantai, escape lain, nesting
  lebih dari 128 level.

## Semantik null dan tipe

| Ekspresi | Hasil |
|---|---|
| variabel/field tidak ada, path melewati non-context | `null` |
| path pada list (`items.sku`) | list hasil proyeksi per elemen |
| `null = null` | `true` |
| `x = null` (x bukan null) | `false` (`!=` kebalikannya) |
| `=`/`!=` beda tipe (`1 = "1"`) | `null` |
| `<` `<=` `>` `>=` number/number, string/string (urutan code point) | boolean |
| `<` `<=` `>` `>=` dengan null, boolean, context, list, atau beda tipe | `null` |
| context/list `=` | perbandingan dalam (deep) |
| `false and x` / `true or x` | `false` / `true` |
| `true and null` / `false or null` | `null` |
| operand `and`/`or` non-boolean | diperlakukan sebagai `null` |
| `not(true)` / `not(false)` / `not(lainnya)` | `false` / `true` / `null` |

Kasus yang belum pasti ditandai `// verify: Camunda 8.8` di kode dan uji, dan menjadi skenario
paritas.

## Nx

`nx run feel-go:build|test|lint|fuzz` (fuzz memakai `FUZZTIME`, default 60s).
