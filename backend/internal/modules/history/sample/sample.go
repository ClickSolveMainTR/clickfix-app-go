// Package sample embeds the documented example export (docs/examples/gecmis-ornek.json).
// Every name, number and address in it is invented.
package sample

import _ "embed"

//go:embed gecmis-ornek.json
var File []byte
