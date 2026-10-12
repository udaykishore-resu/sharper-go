//go:build race

package export

// raceOn is true under -race, where the detector's own
// bookkeeping makes allocation counts meaningless.
const raceOn = true
