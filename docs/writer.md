# INGR Writer

Go implementation of the [INGR file format](https://ingr.io) writer.  
Package: `github.com/ingr-io/ingr-go/ingr`

> **Status:** The writer is in active development. The current API (`RecordsWriter`) is the
> low-level foundation. A higher-level `Encoder` API mirroring `encoding/json` is planned —
> see [implementation-plan.md](implementation-plan.md).

---

## Overview

```
NewRecordsWriter(w, [hashAlg])
    └─ WriteHeader(title)        ← must be called first
    └─ WriteRecords(delimiter, records...)
    └─ Close()                   ← writes the footer; always defer this
```

---

## Creating a writer

```go
func NewRecordsWriter(w io.Writer, hashAlg ...HashAlgorithm) RecordsWriter
```

`w` must be non-nil; passing `nil` panics.

```go
// Basic writer
rw := ingr.NewRecordsWriter(os.Stdout)

// Writer that appends a sha256 hash line to the footer
rw := ingr.NewRecordsWriter(f, ingr.SHA256)
```

The only supported hash algorithm is `ingr.SHA256`.  
Passing an unrecognised algorithm panics.

---

## `RecordsWriter` interface

```go
type RecordsWriter interface {
    WriteRecords(recordsDelimiter int, r ...Record) (n int, err error)
    io.Closer
}
```

### `WriteHeader(title string) (n int, err error)`

Writes the INGR header line to the underlying writer.

```go
rw.(*recordsWriter).WriteHeader("people")
// → # INGR.io | people : col1, col2, ...
```

- Must be called **before** `WriteRecords`.
- Calling it more than once returns an error: `"header already written"`.
- `title` becomes the recordset name in the header.

> **Note:** `WriteHeader` is not part of the `RecordsWriter` interface; it is a method on
> the concrete `*recordsWriter` returned by `NewRecordsWriter`.

### `WriteRecords(recordsDelimiter int, records ...Record) (n int, err error)`

Writes one or more records. Each record's fields are written in the order defined by the column list.

```go
n, err := rw.WriteRecords(0, r1, r2, r3) // no delimiter
n, err := rw.WriteRecords(3, r1, r2)     // appends "#---\n" after each record
```

`recordsDelimiter` controls the optional record separator line:

| Value | Behaviour |
|---|---|
| `0` | No delimiter lines emitted |
| `> 0` | Emits `#` followed by that many `-` characters after each record |

Per the INGR spec, if delimiters are used they must appear after **every** record.  
Calling `WriteRecords` multiple times with `recordsDelimiter > 0` satisfies this automatically as long as every call uses the same non-zero value.

### `Close() error`

Writes the footer and flushes. **Always defer `Close()`** to ensure the footer is written:

```go
rw := ingr.NewRecordsWriter(f)
defer rw.Close()
```

The footer consists of:

1. `# N record(s)\n` — record count line.
2. `# sha256:{hex}` — SHA-256 digest line, only when the writer was created with `ingr.SHA256` (no trailing newline on the last line, per spec).

---

## `ColDef` — column definitions

```go
type ColDef struct {
    Name string `json:"name"`           // required
    Type string `json:"type,omitempty"` // optional type annotation
}
```

`ColDef` describes a column in the INGR header.  
Type annotations follow the INGR spec (e.g. `"string"`, `"int"`, `"decimal"`, `"map[string]any"`).

---

## `Record` interface

`WriteRecords` accepts values that implement `Record`:

```go
type Record interface {
    GetID() string
    GetValue(name string) any
    GetIntValue(name string) int
    GetStrValue(name string) string
    GetBoolValue(name string) bool
    GetComment(name string) string
    GetIsCommentedOut() bool

    SetIsCommentedOut() bool
    SetValue(name string, v any) error
    SetComment(name, value string) error
}
```

### `NewMapRecordEntry` — built-in implementation

```go
func NewMapRecordEntry[TKey comparable](id TKey, data map[string]any) Record
```

Creates a `Record` backed by a `map[string]any`.  
`TKey` is the type of the record key (e.g. `string`, `int`); `GetID()` returns its string representation via `fmt.Sprintf("%v", id)`.

```go
r := ingr.NewMapRecordEntry("alice", map[string]any{
    "name": "Alice Smith",
    "age":  30,
})
fmt.Println(r.GetID())           // "alice"
fmt.Println(r.GetStrValue("name")) // "Alice Smith"
fmt.Println(r.GetIntValue("age"))  // 30
```

> **Known limitation:** `SetIsCommentedOut()` is not yet implemented and panics. Use
> `GetIsCommentedOut()` (read-only) to inspect the flag.

---

## Export options

`ExportOptions` and its functional-option helpers configure serialisation behaviour.  
They are intended for use with higher-level APIs that accept `...ExportOption`.

```go
type ExportOptions struct {
    IncludeHash      bool // append "# sha256:{hex}" to the footer
    RecordsDelimiter bool // write a "#" delimiter line after each record
}
```

### Functional options

```go
// WithHash enables the sha256 hash footer line.
func WithHash() ExportOption

// WithRecordsDelimiter enables a "#" delimiter line after each record.
func WithRecordsDelimiter() ExportOption
```

Build a config with `ApplyOptions`:

```go
opts := ingr.ExportOptions{}
ingr.ApplyOptions(&opts, ingr.WithHash(), ingr.WithRecordsDelimiter())
```

---

## Hash support

Pass `ingr.SHA256` to `NewRecordsWriter` to compute and append a SHA-256 digest to the footer:

```go
f, _ := os.Create("people.ingr")
rw := ingr.NewRecordsWriter(f, ingr.SHA256)
defer rw.Close()
// footer will include: # sha256:3a7bd3e2...
```

The digest covers all bytes written to the underlying writer before `Close` is called (header + records + count line).

---

## Not thread-safe

`RecordsWriter` is **not safe for concurrent use**. Serialise writes externally if needed.

---

## Known limitations

The following items are incomplete in the current implementation:

| Item | Status |
|---|---|
| `WriteHeader` not on interface | Must type-assert to call it |
| Column list not set via public API | `cols` slice is always empty; header writes no column names |
| `WriteRecords` does not increment `recordsCount` | Footer always writes `# 0 record(s)` |
| `SetIsCommentedOut()` | Panics — not yet implemented |
| Footer uses `record(s)` | Spec requires `record` (singular) for N=1 and `records` otherwise |

These will be resolved when the `Encoder` API is introduced.

---

## Planned: `Encoder` API

A higher-level `Encoder` is planned to mirror `encoding/json`:

```go
// Planned — not yet implemented
enc := ingr.NewEncoder(w)
enc.SetHeader(ingr.Header{Recordset: "people", Columns: [...]})
enc.SetDelimiters(true)
enc.SetFooterComments([]string{"sha256:..."})

enc.Encode(person1)
enc.Encode(person2)
enc.Close()
```

Until then, use `NewRecordsWriter` for writing INGR files.
