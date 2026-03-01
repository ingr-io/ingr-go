# INGR Go Library — Implementation Plan

Spec: [ingr-file-format README](https://github.com/ingr-io/ingr-file-format/blob/main/README.md)

Design goal: mirror the ergonomics of `encoding/json` so Go developers feel immediately at home.

---

## Public API Surface

### Analogues to `encoding/json`

| `encoding/json`                                  | `ingr` equivalent                                   | Notes                                             |
|--------------------------------------------------|-----------------------------------------------------|---------------------------------------------------|
| `Marshal(v any) ([]byte, error)`                 | `Marshal(v any) ([]byte, error)`                    | encode records → raw bytes                        |
| `Unmarshal(data []byte, v any) error`            | `Unmarshal(data []byte, v any) error`               | parse raw bytes → records                         |
| `NewEncoder(w io.Writer) *Encoder`               | `NewEncoder(w io.Writer) *Encoder`                  | streaming write                                   |
| `NewDecoder(r io.Reader) *Decoder`               | `NewDecoder(r io.Reader) *Decoder`                  | streaming read                                    |
| `(*Encoder).Encode(v any) error`                 | `(*Encoder).Encode(v any) error`                    | write one record                                  |
| `(*Decoder).Decode(v any) error`                 | `(*Decoder).Decode(v any) error`                    | read one record                                   |
| `(*Decoder).More() bool`                         | `(*Decoder).More() bool`                            | see rationale below                               |
| `json.Valid(data []byte) bool`                   | `ingr.Validate(data []byte) error`                  | see rationale below                               |
| `json.Marshaler` interface                       | `ingr.Marshaler` interface                          | custom per-field JSON encoding hook               |
| `json.Unmarshaler` interface                     | `ingr.Unmarshaler` interface                        | custom per-field JSON decoding hook               |
| `json.RawMessage`                                | `ingr.RawValue`                                     | see rationale below                               |
| `json.Number`                                    | `ingr.Number`                                       | see rationale below                               |
| `(*Decoder).UseNumber()`                         | `(*Decoder).UseNumber()`                            | see rationale below                               |
| `(*Decoder).DisallowUnknownFields()`             | `(*Decoder).DisallowUnknownFields()`                | see rationale below                               |
| `(*Encoder).SetEscapeHTML(bool)`                 | `(*Encoder).SetEscapeHTML(bool)`                    | control HTML escaping in JSON field values        |
| `SyntaxError`, `UnmarshalTypeError`, etc.        | `SyntaxError`, `UnmarshalTypeError`, etc.           | rich error types with line/column position        |

### String convenience wrappers (not in `encoding/json`, justified by INGR use cases)

```go
func MarshalString(v any) (string, error)
func UnmarshalString(s string, v any) error
```

These avoid a `strings.NewReader` / `bytes.Buffer` boilerplate that callers would otherwise write themselves.

### INGR-specific additions

```go
// Header carries the metadata parsed from line 1.
type Header struct {
    Recordset string   // e.g. "people"
    Columns   []string // e.g. ["$ID", "name", "age"]
}

// (*Decoder).Header() returns the parsed header after the first Decode call
// (or after calling ReadHeader explicitly).
func (d *Decoder) Header() Header

// (*Encoder).SetHeader sets the header written on the first Encode call.
func (e *Encoder) SetHeader(h Header)

// (*Encoder).SetDelimiters controls whether bare '#' separator lines are emitted.
func (e *Encoder) SetDelimiters(on bool)

// (*Encoder).SetFooterComments appends extra '#'-prefixed footer lines (e.g. sha256).
func (e *Encoder) SetFooterComments(lines []string)
```

---

## Feature Rationale

### `Validate(data []byte) error` instead of `Valid(data []byte) bool`

`encoding/json` exposes `Valid([]byte) bool` which only answers "is this valid?" with no
diagnostic. For INGR, failure modes are more varied: wrong record count in footer, partial
last record, delimiter used inconsistently, malformed header, etc. A bare `bool` leaves the
caller with nothing actionable. `Validate` returns a `SyntaxError` (with line number) that
pinpoints the problem, making it usable as a lint/CI tool as well as a runtime guard.

### `More() bool` — do we need it?

In `encoding/json`, `More()` lets callers drive a `for` loop without catching `io.EOF`:

```go
for dec.More() {
    dec.Decode(&v)
}
```

For INGR this is less compelling than for JSON: every file has an explicit footer with a
record count, so the decoder knows exactly how many records to expect and `Decode` can
simply return `io.EOF` when done — the same pattern works:

```go
for {
    err := dec.Decode(&v)
    if errors.Is(err, io.EOF) { break }
    ...
}
```

**Verdict:** include `More()` anyway for API symmetry and to make porting JSON-style code
trivial, but it is not essential.

### `UseNumber()` — rationale and use case

By default the decoder unmarshals JSON numbers into `float64`. This is fine for small
integers and low-precision floats, but `float64` has only 53 bits of mantissa, so large
integers (e.g. 64-bit database IDs) or high-precision decimals (e.g. financial amounts)
lose information silently:

```go
// Without UseNumber — precision lost
var row map[string]any
dec.Decode(&row)
fmt.Println(row["id"])  // 9.223372036854776e+18 (wrong!)

// With UseNumber — preserved as the original string
dec.UseNumber()
dec.Decode(&row)
n := row["id"].(ingr.Number)
id, _ := n.Int64()  // 9223372036854775807 (correct)
```

Typical use case: reading an INGR file that was exported from a database where primary keys
are `BIGINT` or amounts are stored as `DECIMAL(18,8)`, then forwarding those values to
another system without touching them — `UseNumber` guarantees round-trip fidelity.

### `DisallowUnknownFields()` — rationale and use case

When decoding into a concrete struct, the decoder by default silently ignores any column
in the file that has no matching struct field. This is lenient and forwards-compatible, but
it can mask bugs.

**How it happens:** a producer adds a new column to an INGR file (e.g. renames `age` →
`age_years`) without updating the consumer's struct. With the default behaviour the field
is silently dropped; the consumer sees every record's `AgeYears` as zero with no error.
With `DisallowUnknownFields()` the decoder returns an error on the first unknown column,
making the schema mismatch explicit:

```go
dec := ingr.NewDecoder(f)
dec.DisallowUnknownFields()
err := dec.Decode(&people)
// → ingr: unknown field "age_years" at line 4
```

Useful in strict pipelines (e.g. data import jobs) where silent data loss is worse than a
hard failure.

### `RawValue` — rationale and use case

`ingr.RawValue` (equivalent to `json.RawMessage`) is a `[]byte` that implements both
`ingr.Marshaler` and `ingr.Unmarshaler`. When a struct field is of type `RawValue`, the
decoder stores the raw JSON bytes of that field verbatim instead of unmarshalling them, and
the encoder writes those bytes as-is.

**Use cases:**

1. **Deferred / conditional parsing.** You read a large INGR file but only need to
   fully parse some fields depending on another field's value:

   ```go
   type Event struct {
       ID      string          `ingr:"$ID"`
       Kind    string          `ingr:"kind"`
       Payload ingr.RawValue   `ingr:"payload"` // parse later based on Kind
   }
   ```

2. **Schema-agnostic proxying.** A gateway reads an INGR file and forwards each record to
   another system. It needs to inspect `$ID` but must not alter or re-encode the other
   fields. Using `RawValue` for every non-key field guarantees bit-perfect pass-through.

3. **Storing heterogeneous JSON.** A column whose value may be a string, object, or array
   depending on the record can be captured as `RawValue` and decoded later with the
   correct target type chosen at runtime.

---

## Struct Tag

```go
type Person struct {
    ID   string  `ingr:"$ID"`
    Name string  `ingr:"name"`
    Age  int     `ingr:"age"`
    Bio  *string `ingr:"bio,omitempty"`
}
```

Supported tag options (mirror `encoding/json`):
- `"-"` — skip field
- `"omitempty"` — emit `null` when zero/nil (same semantics as JSON)

---

## Package Layout

```
ingr-go/
├── ingr.go          – Marshal / Unmarshal / Validate
├── encode.go        – Encoder, encoding logic, struct reflection
├── decode.go        – Decoder, decoding logic, struct reflection
├── header.go        – Header type, header parse/format
├── errors.go        – SyntaxError, UnmarshalTypeError, etc.
├── number.go        – Number type (thin wrapper, same as json.Number)
├── raw.go           – RawValue type (same as json.RawMessage)
├── tags.go          – struct tag parsing helpers
└── ingr_test.go     – table-driven tests
```

---

## Key Implementation Notes

### Parser algorithm (from spec §2)

1. Read line 1 → `parseHeader()` → column list (length `N`).
2. Loop:
   a. Read `N` lines → one record.
   b. If next line is bare `#` → skip (optional delimiter).
   c. If next line matches `# {N} record(s)` → enter footer mode, collect remaining `#` lines, stop.
3. Validate record count against footer count.

### Writer algorithm

1. Write header line.
2. For each record: write `N` JSON-encoded field lines; optionally write `#` delimiter.
3. Write `# {N} record(s)` count line.
4. Write any extra footer comment lines.
5. **No trailing newline** after the last line (per spec §3.6).

### Value encoding

Each field value is serialised as compact single-line JSON using `encoding/json.Marshal`
internally — no embedded newlines permitted. JSON objects/arrays with embedded newlines
must be rejected on decode and compacted on encode.

### Error types

```go
type SyntaxError struct {
    Offset int64  // byte offset in input
    Line   int    // 1-based
    msg    string
}

type UnmarshalTypeError struct {
    Value  string       // JSON value string
    Type   reflect.Type // Go target type
    Offset int64
    Line   int
    Field  string       // column name
}
```

---

## What `encoding/json` has that we are intentionally omitting

| Feature | Reason to omit |
|---|---|
| `MarshalIndent` | Not applicable — INGR is already one-value-per-line |
| `(*Decoder).Token()` / `(*Decoder).Buffered()` | Low-level token stream doesn't map to INGR's fixed-line structure |
| `(*Decoder).InputOffset()` | Can add later; low priority |
| `json.Delim` | INGR has no structural delimiters that need a token type |

---

## Open Questions

1. **Streaming large files**: should `Decoder` expose a `ReadHeader() (Header, error)` method that must be called before the first `Decode`, or should the header be read lazily on the first `Decode`? Lazy is more ergonomic; explicit gives callers earlier error feedback.
2. **sha256 footer**: should `Encoder` compute and emit it automatically (opt-in flag), or leave it to callers via `SetFooterComments`?
3. **Map support**: `encoding/json` accepts `map[string]any`; should we support `map[string]any` for schema-less round-trips? Likely yes.
4. **`[]any` slice target**: allow decoding into `*[]map[string]any` for dynamic use without a concrete struct?
