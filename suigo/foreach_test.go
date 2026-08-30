package suigo

import (
	"os/exec"
	"strings"
	"testing"
)

// TG-213: ForEach の id 引数を省略するとコンパイルが通らない。
// go build を子プロセスで走らせ、失敗することを確認する。
func TestForEachWithoutIDDoesNotCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns go build")
	}
	cmd := exec.Command("go", "build", "-o", t.TempDir()+"/noid", "./testdata/noid")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected compile failure, but build succeeded")
	}
	if !strings.Contains(string(out), "not enough arguments") &&
		!strings.Contains(string(out), "cannot use") {
		t.Errorf("unexpected failure reason:\n%s", out)
	}
}
