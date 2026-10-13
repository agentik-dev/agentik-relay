# Packet refuse-missing-section: a fixture that breaks one rule

| item | value |
|---|---|
| from | `design/relay-r1.md` §2 |
| pin | branch `main` |
| wall | this repo only |
| may touch | `internal/packet/`, `cmd/relay/*.go` (the `plan` case). Nothing else; `internal/yaml/` is read |
| acceptance | `make check; echo $?` reads 0; `ls x \| wc -l` reads 6 or more |
| budget | 10 turns, 20k context, $5 |

## 1. What the source says (quoted)
Quoted.

## 2. Rulings
None.

## 3. Interfaces (signatures, not prose)
None.

## 4. Findings that name this row
None.

None.

## 6. Report (capped)
A table.
