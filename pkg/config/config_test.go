package config_test

import (
	"testing"

	"github.com/rafael/vassal-vlog-sync/pkg/config"
)

func TestExtractInviteToken(t *testing.T) {
	token := "abc-123-def"
	cases := []struct {
		in, want string
	}{
		{token, token},
		{"http://localhost:8080/join/" + token, token},
		{"  " + token + "  ", token},
	}
	for _, c := range cases {
		got := config.ExtractInviteToken(c.in)
		if got != c.want {
			t.Errorf("ExtractInviteToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
