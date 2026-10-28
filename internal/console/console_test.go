package console

import (
	"os"
	"strings"
	"testing"

	"github.com/12312bg/mdbmc/internal/bmc"
)

// TestMain lets the test binary act as the console window process, the way
// the real executable does when started with Flag.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == Flag {
		RunWindow()
		return
	}
	os.Exit(m.Run())
}

func TestChildEnvDropsWebView2Vars(t *testing.T) {
	got := childEnv([]string{"PATH=/bin", "WEBVIEW2_BROWSER_EXECUTABLE_FOLDER=", "webview2_user_data_folder=x", "HOME=/root"})
	if len(got) != 2 || got[0] != "PATH=/bin" || got[1] != "HOME=/root" {
		t.Fatalf("childEnv = %v", got)
	}
}

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

func TestInitScriptLowercasesHost(t *testing.T) {
	s, err := initScript(bmc.LaunchSpec{URL: "https://BMC01.Corp.Local/"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"host":"bmc01.corp.local"`) {
		t.Fatal("host should be lower-cased to match location.host")
	}
}
