# INGR parser

## Header Line

The first line of every `.ingr` file is a metadata header:

```
# INGR.io | {recordset_name}: $ID[:type], col2[:type], col3[:type], ...
```

- The `|` separator between `INGR.io` and the recordset name is **mandatory**.
- Surrounding whitespace around `#` and `|` is optional; parsers must trim it.
- A header missing `|` is a syntax error.

> **Note:** The example in §5 of the upstream INGR file format specification omits the `|`
> (`# INGR.io people: ...`). That is a typo in the spec. The `|` is required by the format
> and this implementation enforces it.

## Inline Comments (proposal §10.1)

This implementation supports the inline comment proposal: any value line may carry an
optional `# comment` suffix after the JSON value, separated by whitespace:

```
true   # enabled by migration
"alice" # primary user
```

Parsers strip everything from the first **unquoted** `#` to the end of the line before
JSON-parsing the value. A `#` inside a JSON string is not a comment start.

