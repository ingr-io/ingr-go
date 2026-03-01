# Package: `github.com/ingr-io/ingr-go/ingr`

Go implementation of INGR file format.

- [Writer](writer.go)
- [Parser](parser.go

## Writer

- Not thread-safe. Use your own synchronization if needed.
- `WriteHeader(title string) (n int, err errror)` must be called first
  before calls to `WriteRecords(records ...Record) (n int, err error)`.

## Parser

Needs design and implementation.