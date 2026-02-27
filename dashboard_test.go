package main

import "testing"

func TestNormalizeAndValidateCNAMETarget(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		want    string
		wantErr bool
	}{
		{name: "adds trailing dot", target: "_abc.sectigo.com", want: "_abc.sectigo.com."},
		{name: "keeps trailing dot", target: "_abc.sectigo.com.", want: "_abc.sectigo.com."},
		{name: "normalizes case and whitespace", target: "  _AbC.AcM-Validations.AWS  ", want: "_abc.acm-validations.aws."},
		{name: "rejects non-authorized domain", target: "example.com", wantErr: true},
		{name: "rejects invalid name", target: "not a domain", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeAndValidateCNAMETarget(tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
