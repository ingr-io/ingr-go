package ingr

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// Marshaler is the interface implemented by types that can marshal themselves
// into a valid INGR field value (single-line JSON).
type Marshaler interface {
	MarshalINGR() ([]byte, error)
}

// Unmarshaler is the interface implemented by types that can unmarshal an INGR
// field value representation of themselves.
type Unmarshaler interface {
	UnmarshalINGR([]byte) error
}

// ---------------------------------------------------------------------------
// Decoder
// ---------------------------------------------------------------------------

// Decoder reads and decodes INGR records from an input stream.
type Decoder struct {
	scanner         *bufio.Scanner
	header          Header
	headerRead      bool
	useNumber       bool
	disallowUnknown bool

	lineNum     int    // 1-based line number of last consumed line
	offset      int64  // approximate byte offset
	peeked      *string // buffered look-ahead line (not yet consumed)
	done        bool    // true once footer is reached or stream exhausted
	recordCount int     // records decoded so far (including commented-out)
	footerCount int     // expected count from footer; -1 = not yet read
	err         error   // sticky error (e.g. footer count mismatch)
}

// NewDecoder returns a new Decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		scanner:     bufio.NewScanner(r),
		footerCount: -1,
	}
}

// UseNumber causes the Decoder to unmarshal JSON numbers into Number
// instead of float64 when the target is an interface{}/any value.
func (d *Decoder) UseNumber() { d.useNumber = true }

// DisallowUnknownFields causes the Decoder to return an error when the
// INGR file contains a column that has no corresponding struct field.
func (d *Decoder) DisallowUnknownFields() { d.disallowUnknown = true }

// Header returns the parsed header. The result is empty until ReadHeader
// or the first Decode call.
func (d *Decoder) Header() Header { return d.header }

// ReadHeader explicitly reads and parses the header line.
// It is safe to call multiple times; only the first call reads from the stream.
func (d *Decoder) ReadHeader() (Header, error) {
	if d.headerRead {
		return d.header, nil
	}
	line, err := d.readLine()
	if err != nil {
		return Header{}, &SyntaxError{
			Line: d.lineNum, Offset: d.offset,
			msg: "unexpected end of input reading header",
		}
	}
	h, parseErr := parseHeader(line)
	if parseErr != nil {
		return Header{}, &SyntaxError{
			Line: d.lineNum, Offset: d.offset,
			msg: parseErr.Error(),
		}
	}
	d.header = h
	d.headerRead = true
	return h, nil
}

func (d *Decoder) ensureHeader() error {
	if !d.headerRead {
		_, err := d.ReadHeader()
		return err
	}
	return nil
}

// More reports whether there is another record in the current INGR stream.
// It returns false when the footer is reached, the stream is exhausted,
// or a sticky error was recorded.
func (d *Decoder) More() bool {
	if d.done || d.err != nil {
		return false
	}
	if err := d.ensureHeader(); err != nil {
		d.err = err
		return false
	}
	line, err := d.peekLine()
	if err != nil {
		// EOF or I/O error → no more records
		d.done = true
		return false
	}
	if isFooterCountLine(line) {
		d.done = true
		if fErr := d.readFooter(); fErr != nil {
			d.err = fErr
		}
		return false
	}
	return true
}

// Decode reads the next INGR record and stores it in v.
//
// Supported target types:
//
//   - *map[string]any        — decode one record into a map
//   - *[]map[string]any      — decode ALL remaining records
//   - *SomeStruct            — decode one record into struct fields via ingr tags
//   - nil                    — skip one record (lines are consumed but not parsed)
//
// Decode returns io.EOF when no more records are available.
func (d *Decoder) Decode(v any) error {
	if d.err != nil {
		return d.err
	}
	if d.done {
		return io.EOF
	}
	if err := d.ensureHeader(); err != nil {
		return err
	}

	// Special case: *[]map[string]any → decode ALL remaining records.
	if v != nil {
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Ptr && !rv.IsNil() &&
			rv.Elem().Kind() == reflect.Slice {
			elemType := rv.Elem().Type().Elem()
			if elemType.Kind() == reflect.Map &&
				elemType.Key().Kind() == reflect.String &&
				elemType.Elem().Kind() == reflect.Interface {
				return d.decodeAll(rv.Elem())
			}
		}
	}

	return d.decodeSingle(v)
}

// ---------------------------------------------------------------------------
// Internal: read all remaining records
// ---------------------------------------------------------------------------

func (d *Decoder) decodeAll(sliceVal reflect.Value) error {
	for d.More() {
		m := make(map[string]any)
		if err := d.decodeSingle(&m); err != nil {
			return err
		}
		sliceVal.Set(reflect.Append(sliceVal, reflect.ValueOf(m)))
	}
	if d.err != nil {
		return d.err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal: decode a single record
// ---------------------------------------------------------------------------

func (d *Decoder) decodeSingle(v any) error {
	numCols := len(d.header.Columns)

	// Peek to see if we've reached the footer (or EOF).
	line, err := d.peekLine()
	if err != nil {
		d.done = true
		return io.EOF
	}
	if isFooterCountLine(line) {
		d.done = true
		if fErr := d.readFooter(); fErr != nil {
			return fErr
		}
		return io.EOF
	}

	// Read N value lines for one record.
	lines := make([]string, numCols)
	lineNums := make([]int, numCols)
	for i := 0; i < numCols; i++ {
		ln, readErr := d.readLine()
		if readErr != nil {
			return &SyntaxError{
				Line: d.lineNum, Offset: d.offset,
				msg: fmt.Sprintf("unexpected end of input: expected %d fields, got %d", numCols, i),
			}
		}
		lines[i] = ln
		lineNums[i] = d.lineNum
	}

	// Detect commented-out record: all or none must be commented.
	commentedCount := 0
	for _, l := range lines {
		if isCommentedOutLine(l) {
			commentedCount++
		}
	}
	if commentedCount > 0 && commentedCount != numCols {
		return &SyntaxError{
			Line: lineNums[0], Offset: d.offset,
			msg: fmt.Sprintf("partially commented-out record: %d of %d lines are commented out",
				commentedCount, numCols),
		}
	}
	isCommented := commentedCount == numCols

	// Strip comment prefix (for commented-out records) and inline comments;
	// produce cleaned JSON byte slices.
	values := make([][]byte, numCols)
	for i, l := range lines {
		raw := l
		if isCommented {
			raw = stripCommentPrefix(raw)
		}
		raw = stripInlineComment(raw)
		raw = strings.TrimSpace(raw)
		if raw == "" {
			raw = "null"
		}
		values[i] = []byte(raw)
	}

	d.recordCount++

	// Consume optional record delimiter.
	if pk, pkErr := d.peekLine(); pkErr == nil && isDelimiter(pk) {
		d.readLine() // discard delimiter
	}

	// nil target → skip.
	if v == nil {
		return nil
	}

	return d.decodeRecord(v, values, lineNums)
}

// ---------------------------------------------------------------------------
// Internal: dispatch to map / struct decoder
// ---------------------------------------------------------------------------

func (d *Decoder) decodeRecord(v any, values [][]byte, lineNums []int) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("ingr: Decode requires a non-nil pointer, got %T", v)
	}

	switch target := v.(type) {
	case *map[string]any:
		return d.decodeIntoMap(target, values, lineNums)
	default:
		elem := rv.Elem()
		if elem.Kind() == reflect.Struct {
			return d.decodeIntoStruct(elem, values, lineNums)
		}
		return fmt.Errorf("ingr: unsupported decode target type %T", v)
	}
}

// ---------------------------------------------------------------------------
// Internal: decode into map[string]any
// ---------------------------------------------------------------------------

func (d *Decoder) decodeIntoMap(target *map[string]any, values [][]byte, lineNums []int) error {
	if *target == nil {
		*target = make(map[string]any)
	}
	for i, col := range d.header.Columns {
		val, err := d.parseJSONValue(values[i])
		if err != nil {
			return &SyntaxError{
				Line: lineNums[i], Offset: d.offset,
				msg: fmt.Sprintf("invalid JSON value for column %q: %v", col.Name, err),
			}
		}
		(*target)[col.Name] = val
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal: decode into struct
// ---------------------------------------------------------------------------

func (d *Decoder) decodeIntoStruct(structVal reflect.Value, values [][]byte, lineNums []int) error {
	fieldMap := buildFieldMap(structVal.Type())

	for i, col := range d.header.Columns {
		fieldIdx, ok := fieldMap[col.Name]
		if !ok {
			if d.disallowUnknown {
				return &SyntaxError{
					Line: lineNums[i], Offset: d.offset,
					msg: fmt.Sprintf("unknown field %q", col.Name),
				}
			}
			continue
		}
		field := structVal.Field(fieldIdx)
		if err := d.decodeIntoField(field, values[i], col.Name, lineNums[i]); err != nil {
			return err
		}
	}
	return nil
}

func (d *Decoder) decodeIntoField(field reflect.Value, data []byte, colName string, lineNum int) error {
	// Check Unmarshaler interface (e.g. RawValue).
	if field.CanAddr() {
		if u, ok := field.Addr().Interface().(Unmarshaler); ok {
			return u.UnmarshalINGR(data)
		}
	}

	// ingr.Number field — store the literal.
	if field.Type() == numberType {
		var n json.Number
		if err := json.Unmarshal(data, &n); err != nil {
			return &UnmarshalTypeError{
				Value: string(data), Type: field.Type(),
				Line: lineNum, Field: colName,
			}
		}
		field.SetString(string(n))
		return nil
	}

	// any / interface{} field — use parseJSONValue (honours UseNumber).
	if field.Kind() == reflect.Interface {
		val, err := d.parseJSONValue(data)
		if err != nil {
			return &SyntaxError{
				Line: lineNum, Offset: d.offset,
				msg: fmt.Sprintf("invalid JSON value for field %q: %v", colName, err),
			}
		}
		if val == nil {
			field.Set(reflect.Zero(field.Type()))
		} else {
			field.Set(reflect.ValueOf(val))
		}
		return nil
	}

	// Default: use encoding/json.Unmarshal into the concrete type.
	ptr := reflect.New(field.Type())
	if err := json.Unmarshal(data, ptr.Interface()); err != nil {
		return &UnmarshalTypeError{
			Value: string(data), Type: field.Type(),
			Line: lineNum, Field: colName,
		}
	}
	field.Set(ptr.Elem())
	return nil
}

// numberType is cached for fast comparison.
var numberType = reflect.TypeOf(Number(""))

// buildFieldMap returns a map from INGR column name → struct field index.
// Tags take precedence; fallback is the lower-cased exported field name.
func buildFieldMap(t reflect.Type) map[string]int {
	m := make(map[string]int, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("ingr")
		if tag == "" {
			m[strings.ToLower(f.Name)] = i
			continue
		}
		ft := parseTag(tag)
		if ft.skip {
			continue
		}
		name := ft.name
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		m[name] = i
	}
	return m
}

// ---------------------------------------------------------------------------
// Internal: JSON value helpers
// ---------------------------------------------------------------------------

// parseJSONValue unmarshals a single-line JSON value into an any.
// When UseNumber is active, json.Number values are converted to ingr.Number.
func (d *Decoder) parseJSONValue(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if d.useNumber {
		dec.UseNumber()
	}
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if d.useNumber {
		v = convertNumbers(v)
	}
	return v, nil
}

// convertNumbers recursively replaces json.Number with ingr.Number.
func convertNumbers(v any) any {
	switch val := v.(type) {
	case json.Number:
		return Number(val.String())
	case map[string]any:
		for k, inner := range val {
			val[k] = convertNumbers(inner)
		}
		return val
	case []any:
		for i, inner := range val {
			val[i] = convertNumbers(inner)
		}
		return val
	default:
		return v
	}
}

// ---------------------------------------------------------------------------
// Internal: line I/O
// ---------------------------------------------------------------------------

// readLine returns the next line from the scanner, consuming the peek buffer
// first. It increments lineNum and offset.
func (d *Decoder) readLine() (string, error) {
	if d.peeked != nil {
		line := *d.peeked
		d.peeked = nil
		d.lineNum++
		d.offset += int64(len(line)) + 1
		return line, nil
	}
	if d.scanner.Scan() {
		d.lineNum++
		line := d.scanner.Text()
		d.offset += int64(len(line)) + 1
		return line, nil
	}
	if err := d.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// peekLine reads the next line without consuming it.
func (d *Decoder) peekLine() (string, error) {
	if d.peeked != nil {
		return *d.peeked, nil
	}
	if d.scanner.Scan() {
		line := d.scanner.Text()
		d.peeked = &line
		return line, nil
	}
	if err := d.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// ---------------------------------------------------------------------------
// Internal: footer
// ---------------------------------------------------------------------------

// readFooter reads and validates the footer. The first footer line
// (# N record(s)) must already be peeked or be the next line in the scanner.
func (d *Decoder) readFooter() error {
	d.done = true

	line, err := d.peekLine()
	if err != nil {
		return nil // no footer present (stream exhausted); tolerate silently
	}
	if !isFooterCountLine(line) {
		return nil
	}
	d.readLine() // consume footer count line

	count := parseFooterCount(line)
	d.footerCount = count
	if count >= 0 && count != d.recordCount {
		return &SyntaxError{
			Line: d.lineNum, Offset: d.offset,
			msg: fmt.Sprintf("footer says %d records but found %d", count, d.recordCount),
		}
	}

	// Consume remaining # comment lines in the footer.
	for {
		pk, pkErr := d.peekLine()
		if pkErr != nil {
			break
		}
		if !strings.HasPrefix(pk, "#") {
			break
		}
		d.readLine()
	}
	return nil
}

// footerCountRe matches the mandatory record-count footer line.
// Requires at least one space/whitespace after '#' to avoid confusion
// with commented-out value lines (which have '#' + value immediately).
var footerCountRe = regexp.MustCompile(`^#\s*(\d+)\s+records?\s*$`)

func isFooterCountLine(line string) bool {
	return footerCountRe.MatchString(line)
}

func parseFooterCount(line string) int {
	m := footerCountRe.FindStringSubmatch(line)
	if m == nil {
		return -1
	}
	n := 0
	for _, ch := range m[1] {
		n = n*10 + int(ch-'0')
	}
	return n
}

// ---------------------------------------------------------------------------
// Internal: line classification helpers
// ---------------------------------------------------------------------------

// isDelimiter checks whether line is a record delimiter (#-..., total len < 80).
func isDelimiter(line string) bool {
	if len(line) < 2 || line[0] != '#' || line[1] != '-' {
		return false
	}
	if len(line) >= 80 {
		return false
	}
	for i := 2; i < len(line); i++ {
		if line[i] != '-' {
			return false
		}
	}
	return true
}

// isCommentedOutLine reports whether line is a commented-out value line.
//
// Rules (from the spec):
//   - '#' alone (end of line)               → commented-out null
//   - '#' followed by non-space, non-dash   → commented-out value
//   - '# ...' (space after #)               → comment / footer line (NOT a value)
//   - '#-...'                                → delimiter (NOT a value)
func isCommentedOutLine(line string) bool {
	if len(line) == 0 || line[0] != '#' {
		return false
	}
	if len(line) == 1 {
		return true // '#' alone = commented-out null
	}
	ch := line[1]
	return ch != ' ' && ch != '-'
}

// stripCommentPrefix removes the leading '#' from a commented-out value line.
func stripCommentPrefix(line string) string {
	if len(line) == 0 || line[0] != '#' {
		return line
	}
	if len(line) == 1 {
		return "" // '#' → empty → will be normalised to "null"
	}
	return line[1:]
}

// stripInlineComment removes an optional inline comment suffix from a value
// line. It correctly handles '#' inside JSON strings:
//
//	`"hello # world"`        → kept (# inside string)
//	`"hello" # comment`      → `"hello"` (# outside string)
//	`true # admin override`  → `true`
func stripInlineComment(line string) string {
	inString := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inString {
			if ch == '\\' {
				i++ // skip escaped character
				continue
			}
			if ch == '"' {
				inString = false
			}
		} else {
			if ch == '"' {
				inString = true
			} else if ch == '#' {
				return strings.TrimRight(line[:i], " \t")
			}
		}
	}
	return line
}
