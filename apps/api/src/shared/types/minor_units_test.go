package types

import (
	"encoding/json"
	"math"
	"testing"
)

func TestMinorUnitsUnmarshalJSONRequiresBoundedDecimalString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    MinorUnits
		wantErr bool
	}{
		{name: "positive", input: `"123"`, want: 123},
		{name: "negative", input: `"-123"`, want: -123},
		{name: "maximum", input: `"9223372036854775807"`, want: MinorUnits(math.MaxInt64)},
		{name: "number rejected", input: `123`, wantErr: true},
		{name: "float rejected", input: `1.5`, wantErr: true},
		{name: "plus rejected", input: `"+1"`, wantErr: true},
		{name: "empty rejected", input: `""`, wantErr: true},
		{name: "decimal rejected", input: `"1.5"`, wantErr: true},
		{name: "exponent rejected", input: `"1e3"`, wantErr: true},
		{name: "overflow rejected", input: `"9223372036854775808"`, wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var amount MinorUnits
			err := json.Unmarshal([]byte(test.input), &amount)
			if test.wantErr && err == nil {
				t.Fatalf("Unmarshal(%s) error = nil, want error", test.input)
			}
			if !test.wantErr && (err != nil || amount != test.want) {
				t.Fatalf("Unmarshal(%s) = (%d, %v), want (%d, nil)", test.input, amount, err, test.want)
			}
		})
	}
}
