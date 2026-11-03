package validate

import "testing"

func TestParseDirectDestinationReadsWhatSshAccepts(t *testing.T) {
	cases := []struct {
		text string
		want DirectDestination
	}{
		{"deploy@10.0.0.5", DirectDestination{User: "deploy", Host: "10.0.0.5"}},
		{"deploy@10.0.0.5:2222", DirectDestination{User: "deploy", Host: "10.0.0.5", Port: 2222}},
		{"deploy@web.example.com", DirectDestination{User: "deploy", Host: "web.example.com"}},
		{"deploy@2001:db8::1", DirectDestination{User: "deploy", Host: "2001:db8::1"}},
		{"deploy@[2001:db8::1]", DirectDestination{User: "deploy", Host: "2001:db8::1"}},
		{"deploy@[2001:db8::1]:2222", DirectDestination{User: "deploy", Host: "2001:db8::1", Port: 2222}},
		{"user@corp.example@web", DirectDestination{User: "user@corp.example", Host: "web"}},
	}
	for _, item := range cases {
		got, ok := ParseDirectDestination(item.text)
		if !ok || got != item.want {
			t.Errorf("ParseDirectDestination(%q) = %+v, %v; want %+v", item.text, got, ok, item.want)
		}
	}
}

func TestParseDirectDestinationRefusesTheRest(t *testing.T) {
	refused := []string{
		"", "web-01", "@host", "deploy@", "deploy@host:", "deploy@host:0", "deploy@host:65536", "deploy@host:+22",
		"deploy@host:http", "deploy@[2001:db8::1", "deploy@[2001:db8::1]2222", "deploy@-host", "deploy@ho st",
		"-oProxyCommand=x@host", "de ploy@host", "de$(id)ploy@host", "de;ploy@host", "de`ploy@host", "de\x7fploy@host",
		"deploy@fe80::1%eth0",
	}
	for _, text := range refused {
		if got, ok := ParseDirectDestination(text); ok {
			t.Errorf("ParseDirectDestination(%q) = %+v, want refused", text, got)
		}
	}
}

func TestParseDirectDestinationRefusesEveryCharacterAShellWouldReadInTheUser(t *testing.T) {
	for _, character := range "'`\";&<>|(){}$\\" {
		text := "de" + string(character) + "ploy@host"
		if got, ok := ParseDirectDestination(text); ok {
			t.Errorf("ParseDirectDestination(%q) = %+v, want refused", text, got)
		}
	}
}

func TestSessionTargetTakesAnAliasOrADirectDestinationOnly(t *testing.T) {
	for _, target := range []string{"web-01", "deploy@10.0.0.5:2222"} {
		if err := SessionTarget(target); err != nil {
			t.Errorf("SessionTarget(%q) = %v", target, err)
		}
	}
	for _, target := range []string{"", "-oProxyCommand=x", "de ploy@host", "web 01"} {
		if err := SessionTarget(target); err == nil {
			t.Errorf("SessionTarget(%q) accepted", target)
		}
	}
}
