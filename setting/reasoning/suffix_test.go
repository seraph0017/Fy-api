package reasoning

import "testing"

func TestParseDeepSeekV4ThinkingSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		modelName  string
		wantBase   string
		wantType   string
		wantEffort string
		wantOK     bool
	}{
		{
			name:       "none suffix",
			modelName:  "deepseek-v4-pro-none",
			wantBase:   "deepseek-v4-pro",
			wantType:   "disabled",
			wantEffort: "",
			wantOK:     true,
		},
		{
			name:       "nothink alias",
			modelName:  "deepseek-v4-pro-nothink",
			wantBase:   "deepseek-v4-pro",
			wantType:   "disabled",
			wantEffort: "",
			wantOK:     true,
		},
		{
			name:       "nothinking alias",
			modelName:  "deepseek-v4-flash-nothinking",
			wantBase:   "deepseek-v4-flash",
			wantType:   "disabled",
			wantEffort: "",
			wantOK:     true,
		},
		{
			name:       "max suffix",
			modelName:  "deepseek-v4-flash-max",
			wantBase:   "deepseek-v4-flash",
			wantType:   "enabled",
			wantEffort: "max",
			wantOK:     true,
		},
		{
			name:       "non deepseek model ignored",
			modelName:  "gpt-4o-nothink",
			wantBase:   "gpt-4o-nothink",
			wantType:   "",
			wantEffort: "",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotBase, gotType, gotEffort, gotOK := ParseDeepSeekV4ThinkingSuffix(tt.modelName)
			if gotBase != tt.wantBase || gotType != tt.wantType || gotEffort != tt.wantEffort || gotOK != tt.wantOK {
				t.Fatalf("ParseDeepSeekV4ThinkingSuffix(%q) = (%q, %q, %q, %v), want (%q, %q, %q, %v)",
					tt.modelName, gotBase, gotType, gotEffort, gotOK,
					tt.wantBase, tt.wantType, tt.wantEffort, tt.wantOK,
				)
			}
		})
	}
}
