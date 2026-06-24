package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig drops a config file in a temp dir and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadConfigValid(t *testing.T) {
	path := writeConfig(t, `
name: demo
groups:
  - name: shared
    services:
      - name: platform
        start: docker compose up -d
        container: my-postgres
    tasks:
      - name: reset
        run: ./reset.sh
        confirm: true
  - name: api
    services:
      - name: backend
        start: npm run dev
        port: 8080
shortcuts:
  - key: g
    label: dashboard
    open: http://localhost:3000
`)

	l, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l.name != "demo" {
		t.Errorf("name = %q, want demo", l.name)
	}
	if len(l.services) != 2 {
		t.Fatalf("services = %d, want 2", len(l.services))
	}

	platform := l.services[0]
	if !platform.docker() {
		t.Error("platform should be docker-style (has container)")
	}
	if platform.fullName != "shared/platform" {
		t.Errorf("fullName = %q, want shared/platform", platform.fullName)
	}
	if platform.logFile != ".devspanner/logs/shared-platform.log" {
		t.Errorf("logFile = %q", platform.logFile)
	}

	backend := l.services[1]
	if backend.docker() {
		t.Error("backend should be process-style (port, no container)")
	}
	if backend.port != 8080 {
		t.Errorf("port = %d, want 8080", backend.port)
	}

	if len(l.tasks) != 1 || l.tasks[0].fullName != "shared/reset" || !l.tasks[0].confirm {
		t.Errorf("tasks = %+v, want one confirm task shared/reset", l.tasks)
	}
	if len(l.scOrder) != 1 || l.scOrder[0] != "g" {
		t.Errorf("scOrder = %v, want [g]", l.scOrder)
	}
}

func TestLoadConfigGlobalTask(t *testing.T) {
	path := writeConfig(t, `
groups:
  - name: api
    services:
      - name: backend
        start: run
tasks:
  - name: deploy
    run: ./deploy.sh
`)
	l, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(l.tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(l.tasks))
	}
	if l.tasks[0].group != "" || l.tasks[0].fullName != "global/deploy" {
		t.Errorf("global task = %+v, want group=\"\" fullName=global/deploy", l.tasks[0])
	}
}

func TestLoadConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string // substring expected in the error
	}{
		{
			name: "group missing name",
			body: "groups:\n  - services:\n      - {name: a, start: x}\n",
			want: "missing a name",
		},
		{
			name: "service missing name",
			body: "groups:\n  - name: g\n    services:\n      - start: x\n",
			want: "missing a name",
		},
		{
			name: "service missing start",
			body: "groups:\n  - name: g\n    services:\n      - name: s\n",
			want: "missing a `start`",
		},
		{
			name: "duplicate service",
			body: "groups:\n  - name: g\n    services:\n      - {name: s, start: x}\n      - {name: s, start: y}\n",
			want: "duplicate service",
		},
		{
			name: "no services",
			body: "groups: []\n",
			want: "no services defined",
		},
		{
			name: "task missing run",
			body: "groups:\n  - name: g\n    services:\n      - {name: s, start: x}\n    tasks:\n      - name: t\n",
			want: "missing a `run`",
		},
		{
			name: "shortcut missing key",
			body: "groups:\n  - name: g\n    services:\n      - {name: s, start: x}\nshortcuts:\n  - {label: l, open: u}\n",
			want: "missing a `key`",
		},
		{
			name: "shortcut reserved key",
			body: "groups:\n  - name: g\n    services:\n      - {name: s, start: x}\nshortcuts:\n  - {key: r, label: l, open: u}\n",
			want: "reserved",
		},
		{
			name: "shortcut both open and run",
			body: "groups:\n  - name: g\n    services:\n      - {name: s, start: x}\nshortcuts:\n  - {key: g, label: l, open: u, run: c}\n",
			want: "exactly one",
		},
		{
			name: "shortcut neither open nor run",
			body: "groups:\n  - name: g\n    services:\n      - {name: s, start: x}\nshortcuts:\n  - {key: g, label: l}\n",
			want: "exactly one",
		},
		{
			name: "duplicate shortcut key",
			body: "groups:\n  - name: g\n    services:\n      - {name: s, start: x}\nshortcuts:\n  - {key: g, label: a, open: u}\n  - {key: g, label: b, open: u}\n",
			want: "duplicate shortcut",
		},
		{
			name: "invalid yaml",
			body: "groups: [unterminated\n",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadConfig(writeConfig(t, tc.body))
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := loadConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if !strings.Contains(err.Error(), "no config") {
		t.Errorf("error %q should mention the missing config", err.Error())
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"shared/platform":  "shared-platform",
		"api/build + test": "api-build-+-test",
		"global/deploy":    "global-deploy",
		"plain":            "plain",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}
