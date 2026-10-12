// Package importer is Technique 69's subject: an importer that,
// like the one in the story, renames its input when done.
package importer

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Import reads "id,amount" lines, then moves the file to .bak.
func Import(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r") // CRLF files
		id, amt, ok := strings.Cut(line, ",")
		if !ok || id == "" {
			f.Close()
			return n, fmt.Errorf("line %d: want id,amount", n+1)
		}
		a, err := strconv.ParseInt(amt, 10, 64)
		if err != nil || a <= 0 {
			f.Close()
			return n, errors.Join(fmt.Errorf("line %d: bad amount", n+1), err)
		}
		n++
	}
	f.Close()
	if err := sc.Err(); err != nil {
		return n, err
	}
	return n, os.Rename(path, path+".bak")
}
