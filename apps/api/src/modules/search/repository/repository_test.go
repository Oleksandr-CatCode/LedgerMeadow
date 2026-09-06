package repository

import "testing"

func TestEscapeLikePrefix(t *testing.T) {
	t.Parallel()
	if got, want := escapeLikePrefix(`50%_off\today`), `50\%\_off\\today`; got != want {
		t.Fatalf("escapeLikePrefix() = %q, want %q", got, want)
	}
}
