package telemetry_test

import (
	"testing"

	"github.com/navigacontentlab/dindenault/internal/telemetry"
)

func TestExtractServiceAndMethod(t *testing.T) {
	tests := []struct {
		name        string
		procedure   string
		wantService string
		wantMethod  string
	}{
		{
			name:        "Normal path",
			procedure:   "test.Service/Method",
			wantService: "test.Service",
			wantMethod:  "Method",
		},
		{
			name:        "Empty path",
			procedure:   "",
			wantService: telemetry.UnknownValue,
			wantMethod:  telemetry.UnknownValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotService, gotMethod := telemetry.ExtractServiceAndMethod(tt.procedure)
			if gotService != tt.wantService {
				t.Errorf("ExtractServiceAndMethod() gotService = %v, want %v", gotService, tt.wantService)
			}

			if gotMethod != tt.wantMethod {
				t.Errorf("ExtractServiceAndMethod() gotMethod = %v, want %v", gotMethod, tt.wantMethod)
			}
		})
	}
}
