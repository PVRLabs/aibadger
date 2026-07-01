package promptpolicy

import "testing"

func TestIsSensitivePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: ".env", want: true},
		{path: ".env.example", want: false},
		{path: ".env.template", want: false},
		{path: ".env.sample", want: false},
		{path: ".ENV.EXAMPLE", want: false},
		{path: ".env.local", want: true},
		{path: ".env.production", want: true},
		{path: ".env.foo", want: true},

		{path: "keys/id_rsa", want: true},
		{path: ".aws/credentials", want: true},
		{path: ".gcp/credentials.json", want: true},
		{path: ".azure/token.json", want: true},
		{path: ".npmrc", want: true},
		{path: "config/service.pem", want: true},
		{path: "config/service.key", want: true},
		{path: "config/credentials", want: true},
		{path: "config/credentials.json", want: true},
		{path: "config/secrets.json", want: true},
		{path: "src/main.go", want: false},
		{path: "assets/logo.png", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsSensitivePath(tt.path)
			if got != tt.want {
				t.Fatalf("IsSensitivePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
