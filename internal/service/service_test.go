package service

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestUnitQuotesPaths(t *testing.T) {
	text := unitFile("/home/pi/go/bin/kando", `/home/pi/my "kan-do" 100%`)
	if !strings.Contains(text, `ExecStart="/home/pi/go/bin/kando" --no-open --home "/home/pi/my \"kan-do\" 100%%"`) {
		t.Fatal(text)
	}
	if !strings.Contains(text, "WantedBy=default.target") {
		t.Fatal(text)
	}
}

func TestPlistIsValidXML(t *testing.T) {
	text := plist("/usr/local/bin/kando", "/Users/a & b/kan-do", "/tmp/k.log")
	if !strings.Contains(text, "/Users/a &amp; b/kan-do") {
		t.Fatal(text)
	}
	d := xml.NewDecoder(strings.NewReader(text))
	d.Strict = true
	for {
		if _, err := d.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("not valid XML: %v\n%s", err, text)
		}
	}
	if strings.Count(text, "/tmp/k.log") != 2 {
		t.Fatal("the log path should be used for both outputs")
	}
}
