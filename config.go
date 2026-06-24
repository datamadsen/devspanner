package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---- config file schema (.devspanner/config.yaml) ----

type configFile struct {
	Name      string         `yaml:"name"` // optional project name shown in the header
	Groups    []groupSpec    `yaml:"groups"`
	Tasks     []taskSpec     `yaml:"tasks"` // global, group-less tasks
	Shortcuts []shortcutSpec `yaml:"shortcuts"`
}

type groupSpec struct {
	Name     string        `yaml:"name"`
	Services []serviceSpec `yaml:"services"`
	Tasks    []taskSpec    `yaml:"tasks"`
}

type serviceSpec struct {
	Name      string `yaml:"name"`
	Dir       string `yaml:"dir"`
	Start     string `yaml:"start"`
	Stop      string `yaml:"stop"`
	Port      int    `yaml:"port"`
	Container string `yaml:"container"`
	Health    string `yaml:"health"`
	Open      string `yaml:"open"`
	Logs      string `yaml:"logs"`
	Watch     string `yaml:"watch"`
}

// A task is a one-shot command (build, deploy, …) — it runs to completion rather
// than staying up, so it lives behind the command palette, not on the dashboard.
type taskSpec struct {
	Name    string `yaml:"name"`
	Dir     string `yaml:"dir"`
	Run     string `yaml:"run"`
	Confirm bool   `yaml:"confirm"`
}

type shortcutSpec struct {
	Key   string `yaml:"key"`
	Label string `yaml:"label"`
	Open  string `yaml:"open"`
	Run   string `yaml:"run"`
}

type loaded struct {
	name      string
	services  []*service
	tasks     []*task
	shortcuts map[string]shortcutSpec
	scOrder   []string
}

// reservedKeys are bound by the console itself; configured shortcuts may not use them.
var reservedKeys = map[string]bool{
	"up": true, "down": true, "j": true, "k": true,
	"enter": true, "esc": true, "l": true, "r": true, "s": true,
	"o": true, "a": true, "x": true, "c": true, "q": true, "ctrl+c": true,
}

// loadConfig reads and validates the console configuration. It returns actionable
// errors so a typo surfaces in the terminal rather than as a broken TUI.
func loadConfig(path string) (*loaded, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no config at %s — create it (see .devspanner/config.yaml in the repo)", path)
		}
		return nil, err
	}

	var cf configFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	l := &loaded{name: strings.TrimSpace(cf.Name), shortcuts: map[string]shortcutSpec{}}
	seenSvc := map[string]bool{}
	for _, g := range cf.Groups {
		if strings.TrimSpace(g.Name) == "" {
			return nil, fmt.Errorf("%s: a group is missing a name", path)
		}
		for _, sp := range g.Services {
			if strings.TrimSpace(sp.Name) == "" {
				return nil, fmt.Errorf("%s: a service in group %q is missing a name", path, g.Name)
			}
			if strings.TrimSpace(sp.Start) == "" {
				return nil, fmt.Errorf("%s: service %q/%q is missing a `start` command", path, g.Name, sp.Name)
			}
			full := g.Name + "/" + sp.Name
			if seenSvc[full] {
				return nil, fmt.Errorf("%s: duplicate service %q", path, full)
			}
			seenSvc[full] = true
			l.services = append(l.services, &service{
				group: g.Name, name: sp.Name, fullName: full,
				dir: sp.Dir, start: sp.Start, stop: sp.Stop, port: sp.Port,
				container: sp.Container, health: sp.Health, open: sp.Open,
				logsCmd: sp.Logs, watch: sp.Watch,
				logFile: ".devspanner/logs/" + sanitize(full) + ".log",
			})
		}
		if err := l.addTasks(path, g.Name, g.Tasks); err != nil {
			return nil, err
		}
	}
	if len(l.services) == 0 {
		return nil, fmt.Errorf("%s: no services defined", path)
	}
	if err := l.addTasks(path, "", cf.Tasks); err != nil {
		return nil, err
	}

	for _, sc := range cf.Shortcuts {
		key := strings.TrimSpace(sc.Key)
		if key == "" {
			return nil, fmt.Errorf("%s: a shortcut is missing a `key`", path)
		}
		if reservedKeys[key] {
			return nil, fmt.Errorf("%s: shortcut key %q is reserved by the console", path, key)
		}
		if seen, ok := l.shortcuts[key]; ok {
			return nil, fmt.Errorf("%s: duplicate shortcut key %q (%s)", path, key, seen.Label)
		}
		if (sc.Open == "") == (sc.Run == "") {
			return nil, fmt.Errorf("%s: shortcut %q must set exactly one of `open` or `run`", path, key)
		}
		l.shortcuts[key] = sc
		l.scOrder = append(l.scOrder, key)
	}

	return l, nil
}

func (l *loaded) addTasks(path, group string, specs []taskSpec) error {
	for _, tp := range specs {
		where := group
		if where == "" {
			where = "global"
		}
		if strings.TrimSpace(tp.Name) == "" {
			return fmt.Errorf("%s: a %s task is missing a name", path, where)
		}
		if strings.TrimSpace(tp.Run) == "" {
			return fmt.Errorf("%s: task %q/%q is missing a `run` command", path, where, tp.Name)
		}
		full := where + "/" + tp.Name
		l.tasks = append(l.tasks, &task{
			group: group, name: tp.Name, fullName: full,
			dir: tp.Dir, run: tp.Run, confirm: tp.Confirm,
			logFile: ".devspanner/logs/task-" + sanitize(full) + ".log",
		})
	}
	return nil
}

// sanitize turns a "group/name" into a filesystem-safe log filename stem.
func sanitize(s string) string {
	return strings.NewReplacer("/", "-", " ", "-").Replace(s)
}
