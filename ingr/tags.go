package ingr

import "strings"

// fieldTag holds the parsed result of an `ingr:"..."` struct tag.
type fieldTag struct {
	name      string // column name override (empty → use lowercase field name)
	omitempty bool   // omit if zero value
	skip      bool   // "-" tag: skip this field entirely
}

// parseTag parses the value of an ingr struct tag.
//
//	parseTag("name")          → fieldTag{name:"name"}
//	parseTag("name,omitempty") → fieldTag{name:"name", omitempty:true}
//	parseTag("-")             → fieldTag{skip:true}
func parseTag(tag string) fieldTag {
	if tag == "-" {
		return fieldTag{skip: true}
	}
	name, opts, _ := strings.Cut(tag, ",")
	ft := fieldTag{name: name}
	if opts == "omitempty" {
		ft.omitempty = true
	}
	return ft
}
