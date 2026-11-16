package gui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSSBuildFailureStopsPackaging(t *testing.T) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make unavailable; CSS build contract not checked")
	}
	for _, target := range []string{"build", "build-ui", "dist", "dist-binaries"} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			write := func(name, text string, mode os.FileMode) {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(text), mode); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"Makefile", "scripts/build-css.sh", "scripts/go-common.sh", "scripts/source-fingerprint.mjs"} {
				data, err := os.ReadFile(filepath.Join("../..", name))
				if err != nil {
					t.Fatal(err)
				}
				write(name, string(data), 0755)
			}
			write("cmd/fixture.go", "package main\n", 0644)
			write("pkg/fixture.go", "package fixture\n", 0644)
			compiler := "#!/bin/sh\nwhile [ $# -gt 0 ]; do\n if [ \"$1\" = '-o' ]; then shift; printf 'partial CSS' > \"$1\"; fi\n shift\ndone\necho 'synthetic compiler failure' >&2\nexit 7\n"
			write("node_modules/.bin/tailwindcss", compiler, 0755)
			write("fake-bin/npx", compiler, 0755)
			write("fake-bin/go", "#!/bin/sh\nprintf 'unexpected go call\\n' >> \"$CSS_TEST_GO_LOG\"\n", 0755)
			write("internal/gui/assets/compiled-tailwind.css", "existing CSS", 0644)
			write("bin/routatic-proxy", "existing binary", 0644)
			cmd := exec.Command(makePath, "-C", root, target, "VERSION=synthetic")
			goLog := filepath.Join(root, "go.log")
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "fake-bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "CSS_TEST_GO_LOG="+goLog)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("CSS failure did not stop %s: %s", target, output)
			}
			if _, err := os.Stat(goLog); !os.IsNotExist(err) {
				t.Fatalf("Go invoked after CSS failure: %s", output)
			}
			if !strings.Contains(string(output), "synthetic compiler failure") {
				t.Fatalf("compiler diagnostics lost: %s", output)
			}
			data, err := os.ReadFile(filepath.Join(root, "internal/gui/assets/compiled-tailwind.css"))
			if err != nil || string(data) != "existing CSS" {
				t.Fatalf("failed compiler replaced old CSS: %q %v", data, err)
			}
		})
	}
}
