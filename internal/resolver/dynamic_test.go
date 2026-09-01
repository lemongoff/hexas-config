package resolver

import "testing"

func TestDynamicParse(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantName       string
		wantFunction   string
		wantRecognized bool
	}{
		{name: "local host", input: "${DYN_LOCAL_HOST}", wantName: "LOCAL_HOST", wantRecognized: true},
		{name: "user name", input: "${DYN_USER_NAME}", wantName: "USER_NAME", wantRecognized: true},
		{name: "workspace", input: "${DYN_WORKSPACE_ROOT}", wantName: "WORKSPACE_ROOT", wantRecognized: true},
		{name: "function", input: "${DYN_WORKSPACE_ROOT.delspace}", wantName: "WORKSPACE_ROOT", wantFunction: "delspace", wantRecognized: true},
		{name: "missing brace", input: "${DYN_WORKSPACE_ROOT.delspace", wantRecognized: false},
		{name: "missing key", input: "${DYN_.delspace}", wantRecognized: false},
		{name: "missing function", input: "${DYN_f.}", wantRecognized: false},
	}

	resolver := dynamic{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, function, found := resolver.parse(test.input)
			if name != test.wantName || function != test.wantFunction || found != test.wantRecognized {
				t.Fatalf("parse(%q) = (%q, %q, %t)", test.input, name, function, found)
			}
		})
	}
}
