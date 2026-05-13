package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/atotto/clipboard"
)

// ── Fixed palette (non-theme colours) ────────────────────────────────────────

var (
	colGray    = lipgloss.Color("#777788")
	colDimGray = lipgloss.Color("#444455")
	colWhite   = lipgloss.Color("#E8E8E8")
	colRed     = lipgloss.Color("#E06C75")
	colGreen   = lipgloss.Color("#98C379")
)

// Fixed styles that never change with the theme.
var (
	styleDir     = lipgloss.NewStyle().Foreground(colGray)
	styleSub     = lipgloss.NewStyle().Foreground(colGray)
	styleMeta    = lipgloss.NewStyle().Foreground(colGray)
	styleNotif   = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	styleError   = lipgloss.NewStyle().Foreground(colRed)
	styleLoading = lipgloss.NewStyle().Foreground(colDimGray).Italic(true)
	styleHint    = lipgloss.NewStyle().Foreground(colGray)
	styleMuted   = lipgloss.NewStyle().Foreground(colDimGray)
)

// ── Color schemes ─────────────────────────────────────────────────────────────

type colorScheme struct {
	color   string         // "Red", "Orange", …
	variant string         // "Dark" or "Light"
	primary lipgloss.Color // title, borders, key hints, count
	dim     lipgloss.Color // selection row background
	file    lipgloss.Color // filename text
	sel     lipgloss.Color // selected-row filename text
}

// themes is laid out as pairs [Dark, Light] per hue in rainbow+pink order.
// Index arithmetic: row = i/2, col = i%2  (col 0=Dark, 1=Light).
var themes = []colorScheme{
	// Red
	{"Red", "Dark", "#D44B5C", "#4A1520", "#F08080", "#FFD7D7"},
	{"Red", "Light", "#FF7F87", "#5C1A22", "#FFBDBD", "#FFE8E8"},
	// Orange
	{"Orange", "Dark", "#D46B1A", "#3D1E00", "#FFA04D", "#FFE0B2"},
	{"Orange", "Light", "#FF9F47", "#4A2600", "#FFD09E", "#FFF3E0"},
	// Yellow
	{"Yellow", "Dark", "#C8A000", "#3D3000", "#FFD740", "#FFF9C4"},
	{"Yellow", "Light", "#F0C848", "#3D3500", "#FFE580", "#FFFDE7"},
	// Green
	{"Green", "Dark", "#2DB86A", "#0D3520", "#69E08A", "#C8F5D8"},
	{"Green", "Light", "#5CDB95", "#0D3D25", "#A8E6C5", "#E8F5E9"},
	// Blue
	{"Blue", "Dark", "#3A86D4", "#0A2040", "#74B9FF", "#BBDEFB"},
	{"Blue", "Light", "#64B5F6", "#0D2545", "#90CAF9", "#E3F2FD"},
	// Purple (default: index 10)
	{"Purple", "Dark", "#9B7FD4", "#3D2F6E", "#5ECCE9", "#F0C060"},
	{"Purple", "Light", "#B39DDB", "#4A3A7A", "#80DEEA", "#E8D5F5"},
	// Pink
	{"Pink", "Dark", "#E91E8C", "#5A0830", "#F48FB1", "#FFD6E8"},
	{"Pink", "Light", "#FF6EB4", "#6B1840", "#FFB3D9", "#FFE6F0"},
}

const defaultThemeIdx = 10 // Purple Dark

// dynStyles holds the theme-dependent styles rebuilt each frame.
type dynStyles struct {
	title     lipgloss.Style
	searchBox lipgloss.Style
	divider   lipgloss.Style
	count     lipgloss.Style
	key       lipgloss.Style
	fileName  lipgloss.Style
	selName   lipgloss.Style
	selDir    lipgloss.Style
	selBg     lipgloss.Style
}

func makeDynStyles(s colorScheme) dynStyles {
	return dynStyles{
		title:     lipgloss.NewStyle().Foreground(s.primary).Bold(true),
		searchBox: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(s.primary).Padding(0, 1),
		divider:   lipgloss.NewStyle().Foreground(s.dim),
		count:     lipgloss.NewStyle().Foreground(s.primary).Bold(true),
		key:       lipgloss.NewStyle().Foreground(s.primary).Bold(true),
		fileName:  lipgloss.NewStyle().Foreground(s.file).Bold(true),
		selName:   lipgloss.NewStyle().Foreground(s.sel).Bold(true).Background(s.dim),
		selDir:    lipgloss.NewStyle().Foreground(colWhite).Background(s.dim),
		selBg:     lipgloss.NewStyle().Background(s.dim),
	}
}

// Theme persistence ────────────────────────────────────────────────────────────

func themeConfigPath() string {
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "anywhere", "theme")
}

func loadSavedTheme() int {
	data, err := os.ReadFile(themeConfigPath())
	if err != nil {
		return defaultThemeIdx
	}
	idx, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || idx < 0 || idx >= len(themes) {
		return defaultThemeIdx
	}
	return idx
}

func saveTheme(idx int) {
	p := themeConfigPath()
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	_ = os.WriteFile(p, []byte(strconv.Itoa(idx)), 0644)
}

// ── Data ──────────────────────────────────────────────────────────────────────

type fileItem struct {
	path string
	name string
	dir  string
}

// ── Messages ──────────────────────────────────────────────────────────────────

type searchDoneMsg struct {
	results []fileItem
	query   string
	gen     int
}
type errMsg struct{ err error }
type clearNotifMsg struct{}

// ── Model ─────────────────────────────────────────────────────────────────────

const (
	itemHeight  = 2
	maxResults  = 500
	headerLines = 2
	searchLines = 3
	divLines    = 1
	statusLines = 2
)

type model struct {
	ti           textinput.Model
	results      []fileItem
	cursor       int
	offset       int
	loading      bool
	err          error
	width        int
	height       int
	query        string
	searchGen    int
	cancelSearch func()
	notif        string
	clipOK       bool
	backendFound bool
	sdkActive    bool

	// theme picker
	themeIdx   int
	pickerOpen bool
	pickerRow  int // 0-6 (hue)
	pickerCol  int // 0=Dark, 1=Light

	// cd-on-exit: path of temp file the shell wrapper reads
	cdFile string
}

func (m model) activeScheme() colorScheme {
	if m.pickerOpen {
		return themes[m.pickerRow*2+m.pickerCol]
	}
	return themes[m.themeIdx]
}

func newModel(clipOK, backendFound, sdkActive bool, themeIdx int, cdFile string) model {
	ti := textinput.New()
	ti.Placeholder = "type to search…"
	ti.Prompt = "  "
	ti.Focus()
	ti.CharLimit = 512
	return model{
		ti:           ti,
		width:        80,
		height:       24,
		clipOK:       clipOK,
		backendFound: backendFound,
		sdkActive:    sdkActive,
		themeIdx:     themeIdx,
		cdFile:       cdFile,
	}
}

// selectedDir returns the directory for the current result: the path itself if
// it is a folder, or the parent directory if it is a file.
func (m model) selectedDir() string {
	if len(m.results) == 0 || m.cursor >= len(m.results) {
		return ""
	}
	path := m.results[m.cursor].path
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return path
	}
	return filepath.Dir(path)
}

// ── Commands ──────────────────────────────────────────────────────────────────

func clearNotifAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearNotifMsg{} })
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m model) Init() tea.Cmd { return textinput.Blink }

// ── Update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ti.Width = m.width - 8

	case searchDoneMsg:
		if msg.gen == m.searchGen {
			m.results = msg.results
			m.loading = false
			m.err = nil
			if m.cursor >= len(m.results) {
				m.cursor, m.offset = 0, 0
			}
		}

	case errMsg:
		m.err = msg.err
		m.loading = false
		m.results = nil

	case clearNotifMsg:
		m.notif = ""

	case cdOpenedMsg:
		m.notif = "✓ terminal opened at " + filepath.Base(string(msg))
		cmds = append(cmds, clearNotifAfter(2*time.Second))

	case tea.KeyMsg:

		// ── Picker-mode keys ──────────────────────────────────────────────────
		if m.pickerOpen {
			switch msg.Type {
			case tea.KeyCtrlC:
				return m, tea.Quit
			case tea.KeyCtrlD:
				if dir := m.selectedDir(); dir != "" {
					return m, m.cdCmd(dir)
				}
			case tea.KeyEsc, tea.KeyCtrlT:
				m.pickerOpen = false
			case tea.KeyUp:
				if m.pickerRow > 0 {
					m.pickerRow--
				}
			case tea.KeyDown:
				if m.pickerRow < len(themes)/2-1 {
					m.pickerRow++
				}
			case tea.KeyLeft:
				m.pickerCol = 0
			case tea.KeyRight:
				m.pickerCol = 1
			case tea.KeyEnter:
				m.themeIdx = m.pickerRow*2 + m.pickerCol
				saveTheme(m.themeIdx)
				m.pickerOpen = false
			}
			return m, nil
		}

		// ── Normal-mode keys ──────────────────────────────────────────────────
		switch msg.Type {

		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyCtrlT:
			m.pickerRow = m.themeIdx / 2
			m.pickerCol = m.themeIdx % 2
			m.pickerOpen = true
			return m, nil

		case tea.KeyEsc:
			if m.ti.Value() == "" {
				return m, tea.Quit
			}
			m.cancelPrev()
			m.ti.SetValue("")
			m.query, m.results, m.err = "", nil, nil
			m.cursor, m.offset = 0, 0
			m.loading = false
			m.searchGen++
			return m, nil

		case tea.KeyUp:
			m.moveCursor(-1)
			return m, nil

		case tea.KeyDown:
			m.moveCursor(1)
			return m, nil

		case tea.KeyPgUp:
			m.moveCursor(-m.visibleCount())
			return m, nil

		case tea.KeyPgDown:
			m.moveCursor(m.visibleCount())
			return m, nil

		case tea.KeyEnter:
			if len(m.results) > 0 && m.cursor < len(m.results) {
				path := m.results[m.cursor].path
				fi, _ := os.Stat(path)
				platformOpenItem(path, fi != nil && fi.IsDir())
				m.notif = "✓ opened"
				cmds = append(cmds, clearNotifAfter(1500*time.Millisecond))
			}
			return m, tea.Batch(cmds...)

		case tea.KeyCtrlO:
			if len(m.results) > 0 && m.cursor < len(m.results) {
				platformRevealItem(m.results[m.cursor].path)
				m.notif = "✓ revealed"
				cmds = append(cmds, clearNotifAfter(2*time.Second))
			}
			return m, tea.Batch(cmds...)

		case tea.KeyCtrlD:
			if dir := m.selectedDir(); dir != "" {
				return m, m.cdCmd(dir)
			}

		case tea.KeyCtrlY:
			if !m.clipOK {
				m.notif = "✗ clipboard unavailable"
				cmds = append(cmds, clearNotifAfter(2*time.Second))
				return m, tea.Batch(cmds...)
			}
			if len(m.results) > 0 && m.cursor < len(m.results) {
				clipboard.WriteAll(m.results[m.cursor].path) //nolint:errcheck
				m.notif = "✓ path copied"
				cmds = append(cmds, clearNotifAfter(2*time.Second))
			}
			return m, tea.Batch(cmds...)
		}

		// All other keys → text input
		prev := m.ti.Value()
		var tiCmd tea.Cmd
		m.ti, tiCmd = m.ti.Update(msg)
		cmds = append(cmds, tiCmd)

		if cur := m.ti.Value(); cur != prev {
			m.cancelPrev()
			m.query = cur
			m.loading = cur != ""
			m.cursor, m.offset = 0, 0
			m.err = nil
			m.searchGen++
			ctx, cancel := context.WithCancel(context.Background())
			m.cancelSearch = cancel
			cmds = append(cmds, platformSearch(ctx, cur, m.searchGen))
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *model) cancelPrev() {
	if m.cancelSearch != nil {
		m.cancelSearch()
		m.cancelSearch = nil
	}
}

// cdCmd handles Ctrl+D:
//   - Shell-wrapper mode (--cd-file set): write dir to the file and quit.
//   - Standalone mode: open a new terminal window at dir.
func (m model) cdCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		if m.cdFile != "" {
			_ = os.WriteFile(m.cdFile, []byte(dir), 0644)
			return tea.Quit()
		}
		platformOpenTerminal(dir)
		return cdOpenedMsg(dir)
	}
}

type cdOpenedMsg string

func (m *model) moveCursor(delta int) {
	if len(m.results) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.results) {
		m.cursor = len(m.results) - 1
	}
	vis := m.visibleCount()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
}

func (m model) visibleCount() int {
	avail := m.height - headerLines - searchLines - divLines - statusLines
	if avail < itemHeight {
		return 1
	}
	return avail / itemHeight
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	var sb strings.Builder
	w := m.width
	dy := makeDynStyles(m.activeScheme())

	// ── Header ────────────────────────────────────────────────────────────────
	left := dy.title.Render("⚡ anywhere") + styleSub.Render("  Anywhere Search TUI")
	var right string
	switch {
	case m.loading:
		right = styleLoading.Render("searching…")
	case len(m.results) > 0:
		suffix := " results"
		if len(m.results) >= maxResults {
			suffix = "+ results"
		}
		right = dy.count.Render(fmt.Sprintf("%d", len(m.results))) + styleSub.Render(suffix)
	case m.sdkActive && !m.pickerOpen:
		right = styleMuted.Render(platformFastLabel())
	}
	sb.WriteString(hline(left, right, w))
	sb.WriteByte('\n')
	sb.WriteString(divLine(dy, w))
	sb.WriteByte('\n')

	// ── Search box ────────────────────────────────────────────────────────────
	sb.WriteString(dy.searchBox.Width(w - 4).Render(m.ti.View()))
	sb.WriteByte('\n')
	sb.WriteString(divLine(dy, w))
	sb.WriteByte('\n')

	// ── Results or Picker ─────────────────────────────────────────────────────
	vis := m.visibleCount()
	if m.pickerOpen {
		sb.WriteString(viewPicker(m, dy, w, vis))
	} else {
		sb.WriteString(viewResults(m, dy, w, vis))
	}

	// ── Status / hints ────────────────────────────────────────────────────────
	sb.WriteString(divLine(dy, w))
	sb.WriteByte('\n')
	meta, hints := buildStatusLine(m, dy)
	gap := w - lipgloss.Width(meta) - lipgloss.Width(hints)
	if gap < 1 {
		gap = 1
	}
	sb.WriteString(meta)
	sb.WriteString(strings.Repeat(" ", gap))
	sb.WriteString(hints)

	return sb.String()
}

// ── Results area ──────────────────────────────────────────────────────────────

func viewResults(m model, dy dynStyles, w, vis int) string {
	var sb strings.Builder
	end := m.offset + vis
	if end > len(m.results) {
		end = len(m.results)
	}
	rendered := 0

	switch {
	case !m.backendFound:
		sb.WriteString(styleError.Render("  ✗ no search backend found"))
		sb.WriteString(styleMuted.Render("\n    " + platformBackendErrorMsg()))
		rendered = 1
	case m.err != nil:
		sb.WriteString(styleError.Render("  ✗ " + m.err.Error()))
		rendered = 1
	case len(m.results) == 0 && m.query != "" && !m.loading:
		sb.WriteString(styleMuted.Render("  no results"))
		rendered = 1
	case m.query == "":
		sb.WriteString(styleMuted.Render("  start typing to search…"))
		rendered = 1
	default:
		for i := m.offset; i < end; i++ {
			item := m.results[i]
			if i == m.cursor {
				sb.WriteString(dy.selBg.Width(w).Render(dy.selName.Render(" ›  " + item.name)))
				sb.WriteByte('\n')
				sb.WriteString(dy.selBg.Width(w).Render(dy.selDir.Render("    " + item.dir)))
			} else {
				sb.WriteString(dy.fileName.Render(" ·  " + item.name))
				sb.WriteByte('\n')
				sb.WriteString(styleDir.Render("    " + item.dir))
			}
			sb.WriteByte('\n')
			rendered++
		}
	}

	for i := rendered; i < vis; i++ {
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// ── Picker area ───────────────────────────────────────────────────────────────

func viewPicker(m model, dy dynStyles, w, vis int) string {
	var sb strings.Builder
	hues := []string{"Red", "Orange", "Yellow", "Green", "Blue", "Purple", "Pink"}
	colW := w / 2
	linesUsed := 0

	// Title row
	title := dy.title.Render("  Color Scheme")
	hint := styleMuted.Render("  ↑↓ ←→ navigate  ·  enter select  ·  esc cancel  ")
	sb.WriteString(hline(title, hint, w))
	sb.WriteByte('\n')
	sb.WriteByte('\n')
	linesUsed += 2

	// Column headers
	darkHdr := styleHint.Render("   Dark")
	lightHdr := styleHint.Render("   Light")
	sb.WriteString(lipgloss.NewStyle().Width(colW).Render(darkHdr))
	sb.WriteString(lightHdr)
	sb.WriteByte('\n')
	linesUsed++

	// Thin sub-divider
	sb.WriteString(dy.divider.Render(strings.Repeat("─", w)))
	sb.WriteByte('\n')
	linesUsed++

	// One row per hue
	for row := range hues {
		darkScheme := themes[row*2]
		lightScheme := themes[row*2+1]

		darkEntry := pickerEntry(darkScheme, dy, m.pickerRow == row && m.pickerCol == 0)
		lightEntry := pickerEntry(lightScheme, dy, m.pickerRow == row && m.pickerCol == 1)

		// Confirm marker for the saved theme
		savedRow, savedCol := m.themeIdx/2, m.themeIdx%2
		if savedRow == row && savedCol == 0 {
			darkEntry += styleMuted.Render(" ●")
		}
		if savedRow == row && savedCol == 1 {
			lightEntry += styleMuted.Render(" ●")
		}

		sb.WriteString(lipgloss.NewStyle().Width(colW).Render(darkEntry))
		sb.WriteString(lightEntry)
		sb.WriteByte('\n')
		linesUsed++
	}

	// Pad to fill the results area
	for linesUsed < vis*itemHeight {
		sb.WriteByte('\n')
		linesUsed++
	}
	return sb.String()
}

func pickerEntry(s colorScheme, dy dynStyles, selected bool) string {
	swatch := lipgloss.NewStyle().Background(s.primary).Render("  ")
	label := lipgloss.NewStyle().Foreground(s.primary).Render("  " + s.color + " " + s.variant)

	marker := "   "
	if selected {
		marker = dy.key.Render(" › ")
	}
	return marker + swatch + label
}

// ── Status bar ────────────────────────────────────────────────────────────────

func buildStatusLine(m model, dy dynStyles) (meta, hints string) {
	// Left: notification or file metadata
	if m.notif != "" {
		meta = styleNotif.Render(" " + m.notif)
	} else if len(m.results) > 0 && m.cursor < len(m.results) && !m.pickerOpen {
		if fi, err := os.Stat(m.results[m.cursor].path); err == nil {
			kind := "file"
			if fi.IsDir() {
				kind = "folder"
			}
			meta = styleMeta.Render(fmt.Sprintf(" %s  ·  %s  ·  %s",
				formatSize(fi.Size()),
				fi.ModTime().Format("2006-01-02 15:04"),
				kind,
			))
		}
	}

	// Right: key hints
	k := func(s string) string { return dy.key.Render(s) }
	h := func(s string) string { return styleHint.Render(s) }
	dot := h("  ·  ")

	if m.pickerOpen {
		hints = h("  ") + k("↑↓") + h(" color") + dot + k("←→") + h(" dark/light") +
			dot + k("enter") + h(" confirm") + dot + k("esc") + h(" cancel  ")
	} else {
		parts := []string{
			k("↑↓") + h(" nav"),
			k("enter") + h(" open"),
			k("ctrl+o") + h(" reveal"),
			k("ctrl+d") + h(" cd"),
		}
		if m.clipOK {
			parts = append(parts, k("ctrl+y")+h(" copy"))
		}
		parts = append(parts,
			k("ctrl+t")+h(" theme"),
			k("esc")+h(" clear"),
			k("ctrl+c")+h(" quit"),
		)
		hints = h("  ") + strings.Join(parts, dot)
	}
	return
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func divLine(dy dynStyles, w int) string {
	return dy.divider.Render(strings.Repeat("─", w))
}

func hline(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func formatSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	// Parse --cd-file <path> (used by the shell wrapper scripts).
	var cdFile string
	args := os.Args[1:]
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--cd-file" {
			cdFile = args[i+1]
			break
		}
	}

	clipOK := clipboard.WriteAll("") == nil
	backendFound, sdkActive := platformInit()
	themeIdx := loadSavedTheme()

	p := tea.NewProgram(
		newModel(clipOK, backendFound, sdkActive, themeIdx, cdFile),
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
