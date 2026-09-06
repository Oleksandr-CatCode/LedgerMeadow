package handlers

import "testing"

func TestOptionalBalance(t *testing.T) {
	t.Parallel()
	missing, err := optionalBalance("")
	if err != nil || missing != nil {
		t.Fatalf("optionalBalance(empty) = %v, %v", missing, err)
	}
	value, err := optionalBalance("500")
	if err != nil || value == nil || *value != 500 {
		t.Fatalf("optionalBalance(valid) = %v, %v", value, err)
	}
	for _, invalid := range []string{"-1", "+1", "0.01", "synthetic-invalid"} {
		if _, err := optionalBalance(invalid); err == nil {
			t.Fatalf("optionalBalance(%q) error = nil", invalid)
		}
	}
}
