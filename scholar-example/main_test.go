package main

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// https://stackoverflow.com/questions/31352239/how-to-test-the-main-package-functions-in-golang
func TestBadArgs(t *testing.T) {
	cmd := exec.Command("./scholar-example", "some", "bad", "args")
	out, err := cmd.CombinedOutput()
	sout := string(out)
	if err != nil && !strings.Contains(sout, "somefunc failed") {
		fmt.Println(sout) // so we can see the full output
		t.Errorf("%v", err)
	}
}
