// Package govulncheck is Technique 4: what govulncheck's exit code
// and modes do and don't tell you.
//
// The test builds the govulncheck pinned in ledgerd's tool block,
// points it at a two-entry vulnerability database written to a
// temp folder (file:// URL, no network), and scans tiny programs.
// The entries are fake: GO-2099-0001 marks strconv.QuoteToGraphic
// and GO-2099-0002 marks strings.Replacer.Replace as vulnerable in
// every Go release before 1.99.
package govulncheck
