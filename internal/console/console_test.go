package console

import (
	"strings"
	"testing"

	"github.com/12312bg/mdbmc/internal/bmc"
)

func TestInitScriptEscapesCredentials(t *testing.T) {
	s, err := initScript(bmc.LaunchSpec{
		URL: "https://10.0.0.5:8443/", AutoLogin: true, Username: "ADMIN", Password: `p"w</script>\`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "__MDBMC_CONFIG__") {
		t.Fatal("placeholder not replaced")
	}
	if !strings.Contains(s, `"host":"10.0.0.5:8443"`) {
		t.Fatal("host missing from config")
	}
	if strings.Contains(s, `p"w`) || strings.Contains(s, "</script>") {
		t.Fatal("password not JSON-escaped")
	}

	s, err = initScript(bmc.LaunchSpec{URL: "https://10.0.0.5/", Username: "ADMIN", Password: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"autologin":false`) {
		t.Fatal("autologin should be off when not requested")
	}
	if _, err := initScript(bmc.LaunchSpec{URL: "not a url"}); err == nil {
		t.Fatal("invalid URL accepted")
	}
}
