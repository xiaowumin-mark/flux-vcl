package flux_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestExternalModuleCompilesDefaultBackend(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("默认 native 后端仅在 Windows 构建")
	}
	repository, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	module := fmt.Sprintf(`module example.com/fluxconsumer

go 1.22

require github.com/xiaowumin-mark/flux-vcl v0.0.0

replace github.com/xiaowumin-mark/flux-vcl => %q
`, filepath.ToSlash(repository))
	source := `package consumer

import (
	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/native"
)

func compilePublicStartup() {
	renderer := native.NewRenderer()
	_ = flux.NewApp(renderer)
	_ = native.Run
	var _ flux.Rect
	var _ *flux.Element
}
`
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "consumer.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "test", "-mod=mod", "-run", "^$", ".")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("外部模块编译超时: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("外部模块无法使用公开默认后端: %v\n%s", err, output)
	}
}
