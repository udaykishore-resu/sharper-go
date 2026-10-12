package intcover

import "testing"

// The unit side of the merge: Validate's error path, which the
// smoke script never reaches.
func TestValidate(t *testing.T) {
	if err := Validate(""); err == nil {
		t.Fatal("empty id accepted")
	}
	if err := Validate("acc_1"); err != nil {
		t.Fatal(err)
	}
}
