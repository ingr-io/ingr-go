# INGR Parser

Go implementation of the [INGR file format](https://ingr.io) parser.  
Package: `github.com/ingr-io/ingr-go/ingr`

---

## Quick start

### Parse a file into a slice of structs

```go
type Person struct {
    ID   string `ingr:"$ID"`
    Name string `ingr:"name"`
    Age  int    `ingr:"age"`
}

f, _ := os.Open("people.ingr")
defer f.Close()

dec := ingr.NewDecoder(f)

var people []map[string]any
dec.Decode(&people) // reads all records at once
```

Or decode record-by-record:

```go
dec := ingr.NewDecoder(f)
for dec.More() {
    var p Person
    if err := dec.Decode(&p); err != nil {
        log.Fatal(err)
    }
    fmt.Println(p.Name)
}
```

### One-shot helpers

```go
// from []byte
var people []map[string]any
err := ingr.Unmarshal(data, &people)

// from string
err := ingr.UnmarshalString(content, &people)

// validation only (no decode target needed)
err := ingr.Validate(data)
```

---

## File format recap

```
# INGR.io | people: $ID:string, name:string, age:int
"alice"
"Alice Smith"
30
#-
"bob"
"Bob Jones"
25
#-
# 2 records
```

- **Line 1** — header: `# INGR.io | {recordset}: col[:type], ...`  
  The `|` separator is **mandatory**.
- **N lines per record** — one JSON value per line (N = number of columns).
- **Optional delimiter** — `#-` (or `#---`) after each record; either all records have one or none do.
- **Commented-out records** — prefix each value line with `#` to soft-delete a record (see §3.8 of the spec).
- **Inline comments** — append `# comment text` after any JSON value (proposal §10.1, enabled by default).
- **Footer** — `# N records` (or `# 1 record`) followed by optional `#`-prefixed lines.

---

## Decoder

### Creating a Decoder

```go
dec := ingr.NewDecoder(r io.Reader) *ingr.Decoder
```

### Reading the header

The header is read lazily on the first `Decode` or `More` call.  
Call `ReadHeader` to read it eagerly and get early error feedback:

```go
h, err := dec.ReadHeader()
// h.Recordset → "people"
// h.Columns   → []ingr.Column{{Name:"$ID", Type:"string"}, {Name:"name", Type:"string"}, ...}
```

After the first call, `ReadHeader` is idempotent — it returns the cached result.  
`dec.Header()` returns the cached header without reading from the stream.

### Decoding records

```go
err := dec.Decode(v any) error
```

Returns `io.EOF` when no more records remain.

**Supported target types:**

| Target type | Behaviour |
|---|---|
| `*map[string]any` | Decodes one record; keys are column names |
| `*[]map[string]any` | Decodes **all** remaining records into the slice |
| `*SomeStruct` | Decodes one record into struct fields via `ingr` tags |
| `nil` | Skips one record (lines are consumed but not parsed) |

### Looping with `More()`

```go
for dec.More() {
    var m map[string]any
    if err := dec.Decode(&m); err != nil {
        log.Fatal(err)
    }
}
```

`More()` returns `false` when the footer line is reached, the stream is exhausted, or a sticky error has been recorded.

---

## Struct tags

Map struct fields to INGR columns with the `ingr` tag:

```go
type Order struct {
    ID       string   `ingr:"$ID"`
    Product  string   `ingr:"product"`
    Qty      int      `ingr:"qty"`
    Note     *string  `ingr:"note,omitempty"`
    Internal string   `ingr:"-"`          // always skipped
}
```

**Tag options:**

| Tag | Meaning |
|---|---|
| `ingr:"colname"` | Use `colname` as the INGR column name |
| `ingr:"colname,omitempty"` | Skip field when zero/nil during encoding |
| `ingr:"-"` | Always skip this field |
| *(no tag)* | Use the lowercase field name |

Column name matching is **case-sensitive**. `$ID` must be written exactly as `$ID` in both the tag and the file header.

---

## Decoder options

### `UseNumber()`

By default, JSON numbers in `any`/`interface{}` fields are decoded as `float64`.  
Call `UseNumber()` before the first `Decode` to preserve them as `ingr.Number` instead — useful for large integers (e.g. 64-bit database IDs) and high-precision decimals:

```go
dec.UseNumber()

var m map[string]any
dec.Decode(&m)

n := m["user_id"].(ingr.Number)
id, _ := n.Int64()    // 9223372036854775807 — no precision loss
f, _ := n.Float64()   // also available
s := n.String()       // original text, e.g. "9223372036854775807"
```

Struct fields typed as `ingr.Number` are always decoded as-is, regardless of `UseNumber`.

### `DisallowUnknownFields()`

By default, columns in the file that have no matching struct field are silently ignored (forward-compatible behaviour).  
Call `DisallowUnknownFields()` to return an error instead:

```go
dec.DisallowUnknownFields()
err := dec.Decode(&p)
// → ingr: unknown field "new_col" (line 5, offset 42)
```

---

## Commented-out records (§3.8)

A record is "commented out" by prefixing every value line with `#` (no space):

```
#"alice"
#"Alice Smith"
#30
```

- `#` alone represents a commented-out `null`.
- **All-or-nothing**: every line of the record must be either commented out or uncommented. A partial mix is a parse error.
- The decoder decodes a commented-out record identically to a normal record — field values are extracted (with the `#` stripped), JSON-parsed, and stored in the target. This means the caller receives the preserved value rather than `null`.

---

## Inline comments (proposal §10.1)

Any value line may carry an optional `# comment` suffix after the JSON value:

```
true   # enabled by migration script on 2024-03-01
"alice" # primary user
42     # calculated from legacy formula
```

The parser strips everything from the first **unquoted** `#` to the end of the line before JSON-parsing. A `#` that appears inside a JSON string is never treated as a comment:

```
"hello # world"           ← kept as-is; the # is inside a string
"active" # set by admin   ← "active" is the value; comment is stripped
```

> This is an early-stage proposal (§10.1 of the INGR spec) and is enabled unconditionally in this implementation.

---

## Custom types

### `ingr.Number`

```go
type Number string

func (n Number) String() string
func (n Number) Float64() (float64, error)
func (n Number) Int64() (int64, error)
```

Use as a struct field type or receive it from `any` fields when `UseNumber()` is active.

### `ingr.RawValue`

`RawValue` is a `[]byte` that defers JSON parsing. It implements `Marshaler` and `Unmarshaler`.

```go
type Event struct {
    ID      string        `ingr:"$ID"`
    Kind    string        `ingr:"kind"`
    Payload ingr.RawValue `ingr:"payload"` // parsed later based on Kind
}
```

`nil` marshals as `null`. Unmarshal stores a copy of the raw JSON bytes verbatim.

### Custom `Marshaler` / `Unmarshaler`

Implement these interfaces to control how a type is encoded/decoded:

```go
type Marshaler interface {
    MarshalINGR() ([]byte, error)
}

type Unmarshaler interface {
    UnmarshalINGR([]byte) error
}
```

---

## Error types

### `*ingr.SyntaxError`

Returned for structural problems (malformed header, partial record, footer count mismatch, invalid JSON value):

```go
type SyntaxError struct {
    Offset int64  // byte offset in the input
    Line   int    // 1-based line number
}
func (e *SyntaxError) Error() string
```

### `*ingr.UnmarshalTypeError`

Returned when a JSON value cannot be assigned to the target Go type:

```go
type UnmarshalTypeError struct {
    Value  string       // JSON representation of the value
    Type   reflect.Type // Go target type
    Offset int64
    Line   int
    Field  string       // INGR column name
}
func (e *UnmarshalTypeError) Error() string
```

Use `errors.As` to inspect error details:

```go
var syntaxErr *ingr.SyntaxError
if errors.As(err, &syntaxErr) {
    fmt.Printf("syntax error at line %d\n", syntaxErr.Line)
}
```

---

## Header type

```go
type Header struct {
    Recordset string
    Columns   []Column
}

type Column struct {
    Name string // e.g. "$ID", "name"
    Type string // optional annotation, e.g. "string", "int"; empty if untyped
}
```

`Header.String()` formats the header back to a valid INGR header line:

```go
h := ingr.Header{
    Recordset: "people",
    Columns: []ingr.Column{
        {Name: "$ID", Type: "string"},
        {Name: "name", Type: "string"},
        {Name: "age", Type: "int"},
    },
}
fmt.Println(h.String())
// → # INGR.io | people: $ID:string, name:string, age:int
```

---

## Validation

`Validate` reads and discards all records, returning the first `*SyntaxError` it encounters:

```go
err := ingr.Validate(data)
if err != nil {
    log.Fatalf("invalid INGR file: %v", err)
}
```

Checks performed:
- Header is well-formed and contains `|`.
- Every value line is a valid single-line JSON expression.
- No partially commented-out records.
- All-or-nothing record delimiter usage.
- Footer record count matches actual records parsed.

---

## Full example

```go
package main

import (
    "errors"
    "fmt"
    "log"
    "os"
    "strings"

    "github.com/ingr-io/ingr-go/ingr"
)

type Person struct {
    ID   string  `ingr:"$ID"`
    Name string  `ingr:"name"`
    Age  int     `ingr:"age"`
    Bio  *string `ingr:"bio,omitempty"`
}

const data = `# INGR.io | people: $ID:string, name:string, age:int, bio
"alice"
"Alice Smith"
30
null
#-
"bob"
"Bob Jones" # added 2024-03-01
25
"Loves Go"
#-
# 2 records`

func main() {
    dec := ingr.NewDecoder(strings.NewReader(data))

    h, err := dec.ReadHeader()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Recordset:", h.Recordset)

    for dec.More() {
        var p Person
        if err := dec.Decode(&p); err != nil {
            var se *ingr.SyntaxError
            if errors.As(err, &se) {
                log.Fatalf("syntax error at line %d: %v", se.Line, err)
            }
            log.Fatal(err)
        }
        fmt.Printf("%s (age %d)\n", p.Name, p.Age)
    }
}
// Output:
// Recordset: people
// Alice Smith (age 30)
// Bob Jones (age 25)
```
