// devspanner — a small, config-driven TUI for the local dev loop.
//
// The dashboard owns the long-running services you juggle while developing locally
// (a docker stack, app backends, frontends) — live status + health, start/stop/
// restart, log tailing, and open-in-browser. One-shot recipes (build, deploy) live
// behind the command palette (press c) so they never clutter the status view. What
// it manages is described in .devspanner/config.yaml at the repo root.
package main

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---- service model ----

// A service is docker-style when it has a container name (liveness = container
// running; start/stop run as commands; logs come from the logs command), and
// otherwise process-style (the console owns the spawned child, captures its
// output, and frees the port before a restart).
type service struct {
	group     string
	name      string // short name within the group, e.g. "backend"
	fullName  string // "group/name" — unique, used for log keys
	dir       string // working dir relative to repo root
	start     string // start command
	stop      string // optional stop command
	port      int    // process-style liveness + free-before-restart
	container string // docker-style liveness
	health    string // optional health URL
	open      string // optional browser URL
	logsCmd   string // optional command that prints logs
	watch     string // optional source dir for stale-build detection
	logFile   string // captured-output file (derived)

	cmd       *exec.Cmd // the child we started (nil if not started by us)
	startedAt time.Time

	// live status (refreshed by the poller)
	running bool
	healthy bool
	owned   bool
	stale   bool
	pid     int
}

func (s *service) docker() bool { return s.container != "" }

// A task is a one-shot command (build, deploy, …). It runs in the background, its
// output is captured to a file and streamed into the log viewer, and it reports an
// exit code when done.
type task struct {
	group    string // "" for global tasks
	name     string
	fullName string // "group/name" (or "global/name")
	dir      string
	run      string
	confirm  bool
	logFile  string

	proc      *exec.Cmd
	running   bool
	done      bool
	exitCode  int
	startedAt time.Time
}

type status struct {
	running, healthy, owned, stale bool
	pid                            int
}

// logSource is anything the log/output viewer can display — a service or a task.
type logSource interface {
	sourceKey() string
	sourceTitle() string
	sourceContent() string
}

// ---- messages ----

type tickMsg struct{}
type statusesMsg map[string]status
type logMsg struct {
	key     string
	content string
}
type taskDoneMsg struct {
	key  string
	code int
}

func tick() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

// ---- helpers ----

var repoRoot string

// version is overwritten at build time via -ldflags "-X main.version=…" (GoReleaser
// does this from the git tag). It stays "dev" for plain `go build`/`go install`.
var version = "dev"

const usage = `devspanner — a config-driven terminal UI for the local dev loop.

Run it from anywhere inside a git repo; it reads .devspanner/config.yaml at the
repo root and takes over the screen. See https://github.com/datamadsen/devspanner.

Usage:
  devspanner            launch the dashboard
  devspanner -v         print the version
  devspanner -h         print this help`

func portPID(port int) int {
	out, err := exec.Command("ss", "-lptnH", fmt.Sprintf("sport = :%d", port)).Output()
	if err != nil {
		return 0
	}
	s := string(out)
	i := strings.Index(s, "pid=")
	if i < 0 {
		return 0
	}
	rest := s[i+4:]
	j := strings.IndexAny(rest, ",")
	if j < 0 {
		return 0
	}
	pid, _ := strconv.Atoi(rest[:j])
	return pid
}

func portListening(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// procEnvHas reports whether process pid was started with key=val in its environment.
// devspanner tags managed services with DEVSPANNER_SERVICE=<fullName>, so this recognises them as
// ours even after devspanner has been closed and reopened (the marker is inherited by children).
func procEnvHas(pid int, key, val string) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return false
	}
	target := key + "=" + val
	for _, kv := range strings.Split(string(data), "\x00") {
		if kv == target {
			return true
		}
	}
	return false
}

// procStartTime returns when pid started, from /proc, so stale-build detection survives
// a devspanner restart (when the in-memory start time is gone). Zero time if it can't be read.
func procStartTime(pid int) time.Time {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return time.Time{}
	}
	// comm (field 2) is parenthesised and may contain spaces — parse after the last ')'.
	rp := strings.LastIndex(string(data), ")")
	if rp < 0 {
		return time.Time{}
	}
	fields := strings.Fields(string(data)[rp+1:])
	// starttime is field 22; fields here begin at field 3 (state), so it's index 19.
	if len(fields) < 20 {
		return time.Time{}
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return time.Time{}
	}
	bt := bootTime()
	if bt.IsZero() {
		return time.Time{}
	}
	const clkTck = 100 // USER_HZ — 100 on effectively all Linux kernels
	return bt.Add(time.Duration(ticks) * time.Second / clkTck)
}

var cachedBootTime time.Time

// bootTime returns the system boot time (invariant, so cached after first read).
func bootTime() time.Time {
	if !cachedBootTime.IsZero() {
		return cachedBootTime
	}
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			sec, err := strconv.ParseInt(strings.TrimSpace(line[len("btime "):]), 10, 64)
			if err != nil {
				return time.Time{}
			}
			cachedBootTime = time.Unix(sec, 0)
			return cachedBootTime
		}
	}
	return time.Time{}
}

func httpOK(url string) bool {
	client := http.Client{Timeout: 600 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode < 500
}

func dockerRunning(name string) bool {
	out, err := exec.Command("docker", "ps", "--filter", "name="+name, "--filter", "status=running", "-q").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func killTree(pid int) {
	if pid <= 0 {
		return
	}
	// Try the whole process group first, then the bare pid.
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)
}

func openBrowser(url string) {
	_ = exec.Command("xdg-open", url).Start()
}

// sh runs a shell command in dir (relative to repo root) and waits for it.
func sh(dir, command string) error {
	c := exec.Command("bash", "-lc", command)
	c.Dir = filepath.Join(repoRoot, dir)
	return c.Run()
}

// runDetached fires a shell command and does not wait — for global shortcuts.
func runDetached(dir, command string) {
	c := exec.Command("bash", "-lc", command)
	c.Dir = filepath.Join(repoRoot, dir)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	_ = c.Start()
}

var skipDirs = map[string]bool{
	"bin": true, "obj": true, "node_modules": true, ".git": true,
	".nuxt": true, ".output": true, "dist": true,
}

// newestMTime returns the most recent modification time of any source file under
// root (skipping build/dependency dirs), used to detect a running build behind source.
func newestMTime(root string) time.Time {
	var newest time.Time
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

// ---- service actions ----

func (s *service) doStart() {
	if s.docker() {
		_ = sh(s.dir, s.start)
		return
	}
	// Free the port if something else holds it (the classic "stale backend" footgun).
	if s.port > 0 {
		if pid := portPID(s.port); pid > 0 {
			killTree(pid)
			time.Sleep(700 * time.Millisecond)
		}
	}
	_ = os.MkdirAll(filepath.Join(repoRoot, ".devspanner", "logs"), 0o755)
	logf, _ := os.Create(filepath.Join(repoRoot, s.logFile))
	cmd := exec.Command("bash", "-lc", s.start)
	cmd.Dir = filepath.Join(repoRoot, s.dir)
	cmd.Stdout = logf
	cmd.Stderr = logf
	// Tag the process (and its children) so we still recognise it as devspanner-managed
	// after devspanner is closed and reopened — the marker lives in its /proc/<pid>/environ.
	cmd.Env = append(os.Environ(), "DEVSPANNER_SERVICE="+s.fullName)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own process group so we can kill the tree
	_ = cmd.Start()
	s.cmd = cmd
	s.startedAt = time.Now()
}

func (s *service) doStop() {
	if s.stop != "" {
		_ = sh(s.dir, s.stop)
	}
	if s.cmd != nil && s.cmd.Process != nil {
		killTree(s.cmd.Process.Pid)
		s.cmd = nil
	}
	if s.port > 0 {
		if pid := portPID(s.port); pid > 0 {
			killTree(pid)
		}
	}
}

func (s *service) restart() {
	s.doStop()
	time.Sleep(500 * time.Millisecond)
	s.doStart()
}

func (s *service) sourceKey() string   { return s.fullName }
func (s *service) sourceTitle() string { return titleStyle.Render("logs · " + s.fullName) }
func (s *service) sourceContent() string {
	if s.logsCmd != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, "bash", "-lc", s.logsCmd)
		c.Dir = filepath.Join(repoRoot, s.dir)
		out, _ := c.CombinedOutput()
		if len(strings.TrimSpace(string(out))) == 0 {
			return dimStyle.Render("(no log output)")
		}
		return readTail(string(out), 500)
	}

	data, err := os.ReadFile(filepath.Join(repoRoot, s.logFile))
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		switch {
		case s.running && !s.owned:
			return warnStyle.Render("(running, but not started by this console — press r to (re)start it here so its log is captured)")
		case !s.running:
			return dimStyle.Render("(not running — press r to start)")
		default:
			return dimStyle.Render("(no log yet — give it a moment)")
		}
	}
	return readTail(string(data), 500)
}

// ---- task actions ----

func (t *task) sourceKey() string { return "task:" + t.fullName }

func (t *task) sourceTitle() string {
	title := titleStyle.Render("task · " + t.fullName)
	switch {
	case t.running:
		return title + warnStyle.Render("  [running]")
	case t.done && t.exitCode == 0:
		return title + okStyle.Render("  [exit 0]")
	case t.done:
		return title + offStyle.Render(fmt.Sprintf("  [exit %d]", t.exitCode))
	}
	return title
}

func (t *task) sourceContent() string {
	data, err := os.ReadFile(filepath.Join(repoRoot, t.logFile))
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return dimStyle.Render("(no output yet)")
	}
	return readTail(string(data), 1000)
}

// launch spawns the task in the background and returns a command that resolves to
// a taskDoneMsg when it exits.
func (t *task) launch() tea.Cmd {
	_ = os.MkdirAll(filepath.Join(repoRoot, ".devspanner", "logs"), 0o755)
	logf, _ := os.Create(filepath.Join(repoRoot, t.logFile))
	cmd := exec.Command("bash", "-lc", t.run)
	cmd.Dir = filepath.Join(repoRoot, t.dir)
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = logf.Close()
		t.running, t.done, t.exitCode = false, true, -1
		return func() tea.Msg { return taskDoneMsg{key: t.fullName, code: -1} }
	}
	t.proc = cmd
	t.running, t.done = true, false
	t.startedAt = time.Now()
	return func() tea.Msg {
		err := cmd.Wait()
		_ = logf.Close()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				code = -1
			}
		}
		return taskDoneMsg{key: t.fullName, code: code}
	}
}

func (t *task) cancel() {
	if t.proc != nil && t.proc.Process != nil {
		killTree(t.proc.Process.Pid)
	}
}

func gather(services []*service) tea.Cmd {
	return func() tea.Msg {
		res := statusesMsg{}
		for _, s := range services {
			st := status{}
			if s.docker() {
				st.running = dockerRunning(s.container)
				if s.health != "" {
					st.healthy = st.running && httpOK(s.health)
				} else {
					st.healthy = st.running
				}
			} else {
				// We own it if we started it this session, or if the process now on
				// the port carries our env marker (i.e. a previous devspanner session started
				// it and it's still running).
				owned := s.cmd != nil && s.cmd.Process != nil
				if s.port > 0 {
					st.pid = portPID(s.port)
					st.running = st.pid > 0 || portListening(s.port)
					if st.pid > 0 && procEnvHas(st.pid, "DEVSPANNER_SERVICE", s.fullName) {
						owned = true
					}
				} else {
					st.running = owned
				}
				st.owned = owned
				if st.running && s.health != "" {
					st.healthy = httpOK(s.health)
				}
				if st.owned && st.running && s.watch != "" {
					// Prefer the process's real start time (from /proc) so this survives
					// a devspanner restart; fall back to the in-session timestamp.
					start := procStartTime(st.pid)
					if start.IsZero() {
						start = s.startedAt
					}
					if !start.IsZero() && newestMTime(filepath.Join(repoRoot, s.watch)).After(start) {
						st.stale = true
					}
				}
			}
			res[s.fullName] = st
		}
		return res
	}
}

// fetchLog resolves a log source's content off the UI thread.
func fetchLog(src logSource) tea.Cmd {
	return func() tea.Msg {
		return logMsg{key: src.sourceKey(), content: src.sourceContent()}
	}
}

func readTail(content string, lines int) string {
	all := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	for i, l := range all {
		all[i] = collapseCR(l)
	}
	return strings.Join(all, "\n")
}

// collapseCR emulates a terminal's handling of carriage returns within a single
// line: a \r rewinds to column 0 and subsequent characters overwrite. This turns
// in-place progress output (e.g. the build timer, docker/npm progress bars) — which
// otherwise piles every frame onto one line and corrupts the viewport — into just
// the final overlaid line.
func collapseCR(line string) string {
	if !strings.ContainsRune(line, '\r') {
		return line
	}
	var buf []rune
	col := 0
	for _, r := range line {
		if r == '\r' {
			col = 0
			continue
		}
		if col < len(buf) {
			buf[col] = r
		} else {
			buf = append(buf, r)
		}
		col++
	}
	return strings.TrimRight(string(buf), " ")
}

// ---- bubbletea model ----

type model struct {
	name       string
	services   []*service
	tasks      []*task
	shortcuts  map[string]shortcutSpec
	scOrder    []string
	cursor     int
	menuCursor int
	confirm    *task
	mode       string // "main" | "logs" | "menu" | "confirm"
	view       logSource
	vp         viewport.Model
	width      int
	height     int
	msg        string
}

func (m model) Init() tea.Cmd {
	return tea.Batch(gather(m.services), tick())
}

func (m *model) openSource(src logSource) tea.Cmd {
	m.view = src
	m.mode = "logs"
	m.vp.SetContent(dimStyle.Render("(loading…)"))
	m.vp.GotoBottom()
	return fetchLog(src)
}

func (m *model) firstTaskOf(group string) int {
	for i, t := range m.tasks {
		if t.group == group {
			return i
		}
	}
	return 0
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp = viewport.New(msg.Width-4, msg.Height-8)

	case tickMsg:
		cmds := []tea.Cmd{gather(m.services), tick()}
		if m.mode == "logs" && m.view != nil {
			cmds = append(cmds, fetchLog(m.view))
		}
		return m, tea.Batch(cmds...)

	case statusesMsg:
		for _, s := range m.services {
			if st, ok := msg[s.fullName]; ok {
				s.running, s.healthy, s.owned, s.stale, s.pid = st.running, st.healthy, st.owned, st.stale, st.pid
			}
		}
		return m, nil

	case logMsg:
		if m.mode == "logs" && m.view != nil && msg.key == m.view.sourceKey() {
			atBottom := m.vp.AtBottom()
			m.vp.SetContent(msg.content)
			if atBottom {
				m.vp.GotoBottom()
			}
		}
		return m, nil

	case taskDoneMsg:
		for _, t := range m.tasks {
			if t.fullName == msg.key {
				t.running, t.done, t.exitCode = false, true, msg.code
				mark := "✓"
				if msg.code != 0 {
					mark = "✗"
				}
				m.msg = fmt.Sprintf("%s %s (exit %d)", t.fullName, mark, msg.code)
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case "logs":
			return m.updateLogs(msg)
		case "menu":
			return m.updateMenu(msg)
		case "confirm":
			return m.updateConfirm(msg)
		default:
			return m.updateMain(msg)
		}
	}
	return m, nil
}

func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sel := m.services[m.cursor]
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.services)-1 {
			m.cursor++
		}
	case "enter", "l":
		return m, m.openSource(sel)
	case "r":
		sel.restart()
		m.msg = fmt.Sprintf("%s: (re)starting…", sel.fullName)
	case "s":
		sel.doStop()
		m.msg = fmt.Sprintf("%s: stopped", sel.fullName)
	case "o":
		if sel.open != "" {
			openBrowser(sel.open)
			m.msg = "opening " + sel.open
		}
	case "a":
		for _, s := range m.services {
			s.restart()
		}
		m.msg = "starting everything…"
	case "x":
		for _, s := range m.services {
			if !s.docker() {
				s.doStop()
			}
		}
		m.msg = "stopped all app processes (infra stays up)"
	case "c":
		if len(m.tasks) == 0 {
			m.msg = "no tasks configured"
		} else {
			m.mode = "menu"
			m.menuCursor = m.firstTaskOf(sel.group)
		}
	default:
		if sc, ok := m.shortcuts[msg.String()]; ok {
			if sc.Open != "" {
				openBrowser(sc.Open)
				m.msg = "opening " + sc.Open
			} else {
				runDetached("", sc.Run)
				m.msg = sc.Label + ": running…"
			}
		}
	}
	return m, nil
}

func (m model) updateLogs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "l":
		m.mode = "main"
		return m, nil
	case "x":
		if t, ok := m.view.(*task); ok && t.running {
			t.cancel()
			m.msg = t.fullName + ": canceling…"
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "c":
		m.mode = "main"
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j":
		if m.menuCursor < len(m.tasks)-1 {
			m.menuCursor++
		}
	case "enter":
		t := m.tasks[m.menuCursor]
		switch {
		case t.running:
			return m, m.openSource(t) // already running — just watch it
		case t.confirm:
			m.confirm = t
			m.mode = "confirm"
		default:
			return m, tea.Batch(t.launch(), m.openSource(t))
		}
	}
	return m, nil
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		t := m.confirm
		m.confirm = nil
		return m, tea.Batch(t.launch(), m.openSource(t))
	case "n", "esc", "q":
		m.confirm = nil
		m.mode = "menu"
	}
	return m, nil
}

// ---- view ----

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	offStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("215"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2).BorderForeground(lipgloss.Color("36"))
)

func (m model) View() string {
	switch m.mode {
	case "logs":
		return m.viewLogs()
	case "menu":
		return m.viewMenu()
	case "confirm":
		return m.viewConfirm()
	default:
		return m.viewMain()
	}
}

func (m model) viewLogs() string {
	hint := "   (↑/↓ scroll · esc back"
	if t, ok := m.view.(*task); ok && t.running {
		hint += " · x cancel"
	}
	hint += ")"
	return m.view.sourceTitle() + dimStyle.Render(hint) + "\n" + m.vp.View()
}

func (m model) viewMain() string {
	// Left column: services grouped by app.
	var b strings.Builder
	group := ""
	for i, s := range m.services {
		if s.group != group {
			if group != "" {
				b.WriteString("\n") // breathing room between groups
			}
			group = s.group
			b.WriteString(headerStyle.Render(group) + "\n")
		}
		cursor := "  "
		if i == m.cursor {
			cursor = selStyle.Render("➤ ")
		}
		b.WriteString(cursor + renderService(s, i == m.cursor) + "\n")
	}
	left := strings.TrimRight(b.String(), "\n")

	// Right column: global shortcuts, stacked top-to-bottom.
	right := lipgloss.NewStyle().MarginLeft(6).Render(m.globalShortcuts())
	cols := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	// Header: the project name (if set) branded with devspanner, else just devspanner.
	title := titleStyle.Render("devspanner")
	if m.name != "" {
		title = titleStyle.Render(m.name) + dimStyle.Render(" · devspanner")
	}
	body := title + "\n\n" + cols + "\n"
	if act := m.activity(); act != "" {
		body += "\n" + act + "\n"
	}
	body += "\n" + m.itemHelp()
	if m.msg != "" {
		body += "\n\n" + dimStyle.Render(m.msg)
	}
	return boxStyle.Render(body)
}

func (m model) viewMenu() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("commands") + dimStyle.Render("   (↑/↓ select · enter run · esc back)") + "\n\n")

	group := "\x00"
	for i, t := range m.tasks {
		g := t.group
		if g == "" {
			g = "global"
		}
		if g != group {
			if group != "\x00" {
				b.WriteString("\n") // breathing room between groups
			}
			group = g
			b.WriteString(headerStyle.Render(g) + "\n")
		}
		cursor := "  "
		if i == m.menuCursor {
			cursor = selStyle.Render("➤ ")
		}
		line := t.name
		if i == m.menuCursor {
			line = selStyle.Render(t.name)
		}
		if t.confirm {
			line += dimStyle.Render("  (confirm)")
		}
		switch {
		case t.running:
			line += warnStyle.Render("  ⟳ running")
		case t.done && t.exitCode == 0:
			line += okStyle.Render("  ✓")
		case t.done:
			line += offStyle.Render(fmt.Sprintf("  ✗ exit %d", t.exitCode))
		}
		b.WriteString("  " + cursor + line + "\n")
	}
	return boxStyle.Render(b.String())
}

func (m model) viewConfirm() string {
	t := m.confirm
	body := titleStyle.Render("confirm") + "\n\n" +
		"Run " + headerStyle.Render(t.fullName) + " ?\n" +
		dimStyle.Render(t.run) + "\n\n" +
		warnStyle.Render("[y]es") + "   " + dimStyle.Render("[n]o")
	return boxStyle.Render(body)
}

// activity summarises tasks running in the background.
func (m model) activity() string {
	var running []string
	for _, t := range m.tasks {
		if t.running {
			running = append(running, t.fullName)
		}
	}
	if len(running) == 0 {
		return ""
	}
	return warnStyle.Render("⟳ " + strings.Join(running, ", "))
}

// globalShortcuts is the right-hand column: built-in global actions plus any
// configured shortcuts, one per line.
func (m model) globalShortcuts() string {
	lines := []string{
		headerStyle.Render("actions"),
		dimStyle.Render("[a] all start"),
		dimStyle.Render("[x] stop apps"),
		dimStyle.Render("[c] commands"),
	}
	for _, key := range m.scOrder {
		if sc, ok := m.shortcuts[key]; ok {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("[%s] %s", key, sc.Label)))
		}
	}
	lines = append(lines, dimStyle.Render("[q] quit"))
	return strings.Join(lines, "\n")
}

// itemHelp is the bottom footer: actions for the selected service. [o]pen only
// shows when the selection actually has a URL.
func (m model) itemHelp() string {
	sel := m.services[m.cursor]
	item := []string{"↑/↓ select", "[r] (re)start", "[s] stop", "[l]ogs"}
	if sel.open != "" {
		item = append(item, "[o]pen in browser")
	}
	return dimStyle.Render(strings.Join(item, "   "))
}

// Fixed column widths so things line up despite the ANSI colour codes:
// name, then the status word, then (if any) the port — in that order.
var nameCol = lipgloss.NewStyle().Width(10)

const statusWidth = 10

// padTo pads s (measuring its visible width, ignoring ANSI) to at least w columns,
// always leaving a gap if it already overflows, so the next column never collides.
func padTo(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s + "  "
}

func renderService(s *service, selected bool) string {
	var dot, label string
	switch {
	case !s.running:
		dot, label = offStyle.Render("○"), offStyle.Render("stopped")
	case s.docker():
		dot, label = okStyle.Render("●"), okStyle.Render("up")
	case s.health != "" && !s.healthy:
		dot, label = warnStyle.Render("◐"), warnStyle.Render("starting…")
	case !s.owned:
		// running but not started by this console — could be a stale/older build
		dot, label = warnStyle.Render("●"), warnStyle.Render("running ")+dimStyle.Render("(external — press r)")
	case s.stale:
		// owned, but source changed since it started — needs a restart to pick up edits
		dot, label = warnStyle.Render("●"), warnStyle.Render("running ")+dimStyle.Render("(code changed — press r)")
	default:
		dot, label = okStyle.Render("●"), okStyle.Render("running")
	}

	name := nameCol.Render(s.name)
	if selected {
		name = nameCol.Bold(true).Foreground(lipgloss.Color("215")).Render(s.name)
	}

	row := dot + " " + name + label
	if s.port > 0 {
		row = dot + " " + name + padTo(label, statusWidth) + dimStyle.Render(fmt.Sprintf(":%d", s.port))
	}
	return row
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Println("devspanner " + version)
			return
		case "-h", "--help", "help":
			fmt.Println(usage)
			return
		default:
			fmt.Fprintln(os.Stderr, "devspanner: unknown argument "+os.Args[1])
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(2)
		}
	}

	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		repoRoot = strings.TrimSpace(string(root))
	} else {
		repoRoot, _ = os.Getwd()
	}

	cfg, err := loadConfig(filepath.Join(repoRoot, ".devspanner", "config.yaml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "devspanner: "+err.Error())
		os.Exit(1)
	}

	m := model{
		name:      cfg.name,
		services:  cfg.services,
		tasks:     cfg.tasks,
		shortcuts: cfg.shortcuts,
		scOrder:   cfg.scOrder,
		mode:      "main",
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
