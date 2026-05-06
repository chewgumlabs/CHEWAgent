package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/gum"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/planner"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/project"
	chewsprite "github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/sprite"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/tool"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/wizard"
)

const (
	tuiMascotRows     = 16
	tuiHeaderRows     = tuiMascotRows + 1
	tuiMinWidth       = 40
	tuiMinHeight      = 20
	gumBlipDuration   = 1500 * time.Millisecond
	gumRenderInterval = 450 * time.Millisecond
)

type tuiApp struct {
	p     *planner.ScriptedPlanner
	tools *tool.Registry
	state *mascotState
	work  *workStatus

	brain        atomic.Pointer[wizard.Brain]
	proj         atomic.Pointer[project.Project]
	sess         *chatSession
	gumKey       gum.Key
	gumKeyActive bool
	gumKeyEnv    bool
	gumKeyErr    string

	brainDir         string
	brainInstalled   bool
	installWizard    *wizard.InstallBrain
	brainResults     chan brainResult
	gumStatusResults chan gumStatusResult
	brainBusy        bool
	gumStatusBusy    bool

	transcript []string
	input      []rune
	scroll     int
	width      int
	height     int

	gumBlipUntil time.Time
}

type brainResult struct {
	input string
	reply string
	err   error
}

type gumStatusResult struct {
	input  string
	output string
	err    error
}

func runTUI() error {
	if !stdinIsTerminal() || !stdoutIsTerminal() {
		return errors.New("stdin/stdout are not terminals")
	}
	repairTerminalViewport()

	raw, err := enterRawTerminal()
	if err != nil {
		return err
	}
	defer raw.restore()

	app := newTUIApp()
	defer app.stopBrain()

	enterTUIScreen()
	defer leaveTUIScreen()

	app.startup()
	app.render()

	inputCh := make(chan []byte, 16)
	errCh := make(chan error, 1)
	go readTUIInput(inputCh, errCh)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		app.stopBrain()
		raw.restore()
		leaveTUIScreen()
		fmt.Println("*splash*")
		os.Exit(0)
	}()

	tick := time.NewTicker(gumRenderInterval)
	defer tick.Stop()

	for {
		select {
		case data := <-inputCh:
			if app.handleInputBytes(data) {
				return nil
			}
		case result := <-app.brainResults:
			app.finishBrainAsk(result)
		case result := <-app.gumStatusResults:
			app.finishGumKeyStatus(result)
		case <-tick.C:
			w, h := terminalSize()
			gumExpired := app.clearExpiredGumBlip()
			if w != app.width || h != app.height || gumExpired || app.gumBlipActive() || app.brainBusy || app.gumStatusBusy || time.Since(app.state.lastTick) > 800*time.Millisecond {
				app.state.advance()
				app.render()
			}
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
}

func newTUIApp() *tuiApp {
	return &tuiApp{
		p:                planner.NewScriptedPlanner(),
		tools:            tool.NewDefault(),
		state:            newMascotState("idle"),
		work:             newWorkStatus(),
		brainResults:     make(chan brainResult, 1),
		gumStatusResults: make(chan gumStatusResult, 1),
	}
}

func (a *tuiApp) startup() {
	a.loadGumKey()
	brainState, brainDir := wizard.CheckBrain()
	a.brainDir = brainDir
	a.brainInstalled = brainState == wizard.BrainNapping
	if brainDir != "" {
		_ = wizard.KillStaleBrain(brainDir)
	}
	switch brainState {
	case wizard.BrainNapping:
		a.appendBlock("Brain installed but napping. Type 'wake up' to load it.")
		a.p.SetFallback(brainNappingFallback(a.p))
	case wizard.BrainNotInstalled:
		a.appendBlock("No brain installed yet. Type 'install brain' to set one up.")
	}
	a.appendBlock(chatIntroText())
	if a.gumKeyActive && a.gumKeyEnv {
		a.appendBlock("Gum key: " + a.gumKey.Label())
	} else if a.gumKeyErr != "" {
		a.appendBlock("Gum key problem: " + a.gumKeyErr)
	}

	if last := project.LoadLast(brainDir); last != "" {
		if pj, err := project.Open(last); err == nil {
			a.proj.Store(pj)
			a.tools.SetRoot(pj.Path)
			a.appendBlock(fmt.Sprintf(a.p.PickVoice("project_resumed"), pj.Name))
		}
	}
}

func readTUIInput(inputCh chan<- []byte, errCh chan<- error) {
	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			errCh <- err
			return
		}
		if n == 0 {
			continue
		}
		cp := make([]byte, n)
		copy(cp, buf[:n])
		inputCh <- cp
	}
}

func enterTUIScreen() {
	fmt.Print("\033[?1049h\033[?25l\033[2J\033[H\033[r")
}

func leaveTUIScreen() {
	fmt.Print("\033[0m\033[?25h\033[r\033[?1049l")
}

func (a *tuiApp) handleInputBytes(data []byte) bool {
	for i := 0; i < len(data); i++ {
		b := data[i]
		switch b {
		case 0x03: // Ctrl+C
			a.appendBlock("*splash*")
			a.render()
			return true
		case 0x04: // Ctrl+D
			if len(a.input) == 0 {
				return true
			}
		case 0x0c: // Ctrl+L
			a.render()
		case 0x15: // Ctrl+U
			a.input = nil
			a.render()
		case '\r', '\n':
			if a.submitInput() {
				return true
			}
		case 0x7f, 0x08:
			if len(a.input) > 0 {
				a.input = a.input[:len(a.input)-1]
				a.render()
			}
		case 0x1b:
			next := a.consumeEscape(data[i:])
			if next > 0 {
				i += next - 1
			}
		default:
			if b < 0x20 {
				continue
			}
			r, size := utf8.DecodeRune(data[i:])
			if r == utf8.RuneError && size == 1 {
				continue
			}
			a.input = append(a.input, r)
			i += size - 1
			a.render()
		}
	}
	return false
}

func (a *tuiApp) consumeEscape(data []byte) int {
	if len(data) < 3 || data[1] != '[' {
		return 1
	}
	for i := 2; i < len(data); i++ {
		c := data[i]
		if (c >= 'A' && c <= 'Z') || c == '~' {
			a.handleEscape(string(data[:i+1]))
			a.render()
			return i + 1
		}
	}
	return len(data)
}

func (a *tuiApp) handleEscape(seq string) {
	page := max(1, a.viewportHeight()-1)
	switch seq {
	case "\033[A":
		a.scrollBy(1)
	case "\033[B":
		a.scrollBy(-1)
	case "\033[5~":
		a.scrollBy(page)
	case "\033[6~":
		a.scrollBy(-page)
	case "\033[H", "\033[1~":
		a.scrollToTop()
	case "\033[F", "\033[4~":
		a.scroll = 0
	}
}

func (a *tuiApp) submitInput() bool {
	input := strings.TrimSpace(string(a.input))
	a.input = nil
	if input == "" {
		a.render()
		return false
	}
	a.appendBlock("> " + input)
	a.render()
	if a.answerForegroundStatus(input) {
		a.render()
		return false
	}
	if a.brainBusy || a.gumStatusBusy {
		a.appendBlock("Still thinking. Ask 'what are you doing?' for the checkpoint, or let this answer land first.")
		a.render()
		return false
	}
	quit := a.runTurn(input)
	if !quit && a.state.current == "walk" {
		a.state.set("idle")
	}
	a.render()
	return quit
}

func (a *tuiApp) answerForegroundStatus(input string) bool {
	if isWorkStatusQuestion(input) || (a.brainBusy && isBareStatusQuestion(input)) {
		if !a.brainBusy && !a.gumStatusBusy && a.gumKeyActive && len(a.gumKey.StatusCommand) > 0 {
			a.startGumKeyStatus(input)
			return true
		}
		a.appendBlock(a.work.Answer(a.proj.Load(), a.brain.Load() != nil))
		return true
	}
	return false
}

func (a *tuiApp) runTurn(input string) bool {
	if a.installWizard != nil {
		done, _ := a.installWizard.Step(input, a.reply)
		if done {
			if newBrain := a.installWizard.RunningBrain(); newBrain != nil {
				a.stopBrain()
				a.brain.Store(newBrain)
				a.brainInstalled = true
				a.refreshTUIBrainSession()
			}
			a.installWizard = nil
			a.state.set("idle")
		}
		return false
	}

	plan := a.p.Plan(input)
	if plan.Mascot != "" {
		a.state.set(plan.Mascot)
		a.render()
	}
	if plan.Response != "" {
		a.appendBlock(plan.Response)
		a.render()
	}

	for _, v := range plan.Verbs {
		a.blipGum()
		a.render()
		a.work.Start("running "+v.Name, "Gum is dispatching a local tool.", a.proj.Load(), a.brain.Load())
		res, err := a.tools.Dispatch(v.Name, v.Params)
		switch {
		case errors.Is(err, tool.ErrUnknownTool):
			a.appendBlock(fmt.Sprintf("(verb planned but not wired: %s %v)", v.Name, v.Params))
			a.work.Fail("Tool was planned but not wired: "+v.Name, a.proj.Load())
		case err != nil:
			a.appendBlock(fmt.Sprintf("(error running %s: %v)", v.Name, err))
			a.state.set("ghost")
			a.work.Fail(fmt.Sprintf("%s failed: %v", v.Name, err), a.proj.Load())
		default:
			if res.Output != "" {
				a.appendBlock(res.Output)
			}
			if res.Mascot != "" {
				a.state.set(res.Mascot)
			}
			a.work.Fact("Tool finished: "+v.Name, a.proj.Load())
			a.work.Finish("Tool finished: "+v.Name, a.proj.Load())
		}
		a.render()
	}

	if plan.Halt {
		return true
	}

	if plan.LaunchWizard != "" {
		a.runSystemAction(plan)
		if !a.brainBusy {
			a.state.set("idle")
		}
	}
	return false
}

func (a *tuiApp) runSystemAction(plan planner.Plan) {
	switch plan.LaunchWizard {
	case "ask_brain":
		a.startBrainAsk(plan.LaunchArgs["input"])
	case "install_brain":
		a.installWizard = wizard.NewInstallBrain()
		a.installWizard.Begin(a.reply)
	case "wake_brain":
		handleWakeBrainWithReply(a.p, &a.brain, &a.proj, a.reply)
		if a.brain.Load() != nil {
			a.brainInstalled = true
			a.refreshTUIBrainSession()
		}
	case "nap_brain":
		handleNapBrainWithReply(a.p, &a.brain, a.brainDir, a.reply)
		a.sess = nil
	case "brain_status":
		handleBrainStatusWithReply(a.p, &a.brain, a.brainDir, a.reply)
	case "profile_status":
		handleProfileStatusWithReply(a.p, a.brainDir, a.reply)
	case "profile_list":
		handleProfileListWithReply(a.p, a.brainDir, a.reply)
	case "profile_use":
		handleProfileUseWithReply(a.p, &a.brain, a.brainDir, plan.LaunchArgs["name"], a.reply)
		a.brainInstalled = wizard.CheckBrainAt(a.brainDir) == wizard.BrainNapping
		setFallbackForBrainState(a.p, a.brainDir)
		a.sess = nil
	case "open_project":
		a.blipGum()
		a.render()
		handleOpenProjectWithReply(a.p, a.tools, &a.proj, &a.brain, a.brainDir, plan.LaunchArgs["path"], a.reply)
		a.refreshTUIBrainSession()
	case "create_project":
		a.blipGum()
		a.render()
		handleCreateProjectWithReply(a.p, a.tools, &a.proj, &a.brain, a.brainDir, plan.LaunchArgs["name"], a.reply)
		a.refreshTUIBrainSession()
	case "create_folder":
		a.blipGum()
		a.render()
		handleCreateFolderWithReply(a.p, plan.LaunchArgs["name"], a.reply)
	case "forget_project":
		a.blipGum()
		a.render()
		handleForgetProjectWithReply(a.p, a.tools, &a.proj, &a.brain, a.brainDir, a.reply)
		a.refreshTUIBrainSession()
	case "remember_note":
		a.blipGum()
		a.render()
		handleRememberNoteWithReply(a.p, &a.proj, &a.brain, plan.LaunchArgs["note"], a.reply)
		a.refreshTUIBrainSession()
	default:
		a.appendBlock(fmt.Sprintf("(unknown wizard requested: %s)", plan.LaunchWizard))
	}
}

func (a *tuiApp) refreshTUIBrainSession() {
	b := a.brain.Load()
	if b == nil {
		a.sess = nil
		return
	}
	a.sess = newChatSession(b, a.proj.Load())
	a.p.SetFallback(func(input string) planner.Plan {
		return planner.Plan{
			LaunchWizard: "ask_brain",
			LaunchArgs:   map[string]string{"input": input},
			Mascot:       "ghost",
		}
	})
}

func (a *tuiApp) loadGumKey() {
	key, fromEnv, err := gum.ActiveKey()
	if err != nil {
		a.gumKeyErr = err.Error()
		return
	}
	a.gumKey = key
	a.gumKeyActive = true
	a.gumKeyEnv = fromEnv
}

func (a *tuiApp) startGumKeyStatus(input string) {
	if a.gumStatusBusy {
		a.appendBlock(a.work.Answer(a.proj.Load(), a.brain.Load() != nil))
		return
	}
	a.gumStatusBusy = true
	a.blipGum()
	a.state.set("ghost")
	a.work.Start("checking "+a.gumKey.Label(), "Gum key status provider in progress.", a.proj.Load(), a.brain.Load())
	a.work.Fact("Asked "+a.gumKey.Label()+" for orientation.", a.proj.Load())
	a.appendBlock("Checking Gum. Hmph. Hold on...")
	a.render()
	key := a.gumKey
	go func() {
		out, err := runGumKeyStatusCommand(key)
		a.gumStatusResults <- gumStatusResult{input: input, output: out, err: err}
	}()
}

func (a *tuiApp) finishGumKeyStatus(result gumStatusResult) {
	a.gumStatusBusy = false
	if result.err != nil {
		a.work.Fail("Gum key status failed: "+result.err.Error(), a.proj.Load())
		a.appendBlock("Gum status failed: " + result.err.Error())
		a.state.set("ghost")
		a.render()
		return
	}
	a.work.Finish("Gum status answered.", a.proj.Load())
	a.appendBlock(result.output)
	a.state.set("idle")
	a.render()
}

func (a *tuiApp) startBrainAsk(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}
	if a.brainBusy {
		a.appendBlock(a.work.Answer(a.proj.Load(), a.brain.Load() != nil))
		return
	}
	b := a.brain.Load()
	if b == nil {
		a.appendBlock(a.p.PickVoice("brain_not_installed"))
		setFallbackForBrainState(a.p, a.brainDir)
		return
	}
	if captureIntentFromChat(a.proj.Load(), input) {
		a.refreshTUIBrainSession()
	}
	if a.sess == nil {
		a.refreshTUIBrainSession()
	}
	sess := a.sess
	if sess == nil {
		a.appendBlock("Hmph. Brain session is missing. Try 'wake up' again.")
		return
	}
	a.brainBusy = true
	a.state.set("ghost")
	a.work.Start(input, "Brain call in progress.", a.proj.Load(), b)
	a.work.Fact("Sent request to "+b.Alias()+".", a.proj.Load())
	a.appendBlock("Thinking. Ask 'what are you doing?' if you want the checkpoint while I work.")
	a.render()
	go func() {
		reply, err := sess.ask(input)
		a.brainResults <- brainResult{input: input, reply: reply, err: err}
	}()
}

func (a *tuiApp) finishBrainAsk(result brainResult) {
	a.brainBusy = false
	if result.err != nil {
		a.work.Fail("Brain did not answer: "+result.err.Error(), a.proj.Load())
		a.appendBlock(fmt.Sprintf("Hmph. Brain didn't answer: %v", result.err))
		a.state.set("ghost")
		a.render()
		return
	}
	a.work.Fact("Brain answered.", a.proj.Load())
	a.work.Finish("Brain answered.", a.proj.Load())
	a.appendBlock(result.reply)
	a.state.set("idle")
	a.render()
}

func (a *tuiApp) blipGum() {
	a.gumBlipUntil = time.Now().Add(gumBlipDuration)
}

func (a *tuiApp) gumBlipActive() bool {
	return !a.gumBlipUntil.IsZero() && time.Now().Before(a.gumBlipUntil)
}

func (a *tuiApp) clearExpiredGumBlip() bool {
	if a.gumBlipUntil.IsZero() || time.Now().Before(a.gumBlipUntil) {
		return false
	}
	a.gumBlipUntil = time.Time{}
	return true
}

func (a *tuiApp) reply(s string) {
	a.appendBlock(s)
	a.render()
}

func (a *tuiApp) stopBrain() {
	if b := a.brain.Swap(nil); b != nil {
		_ = b.Stop()
	}
}

func (a *tuiApp) appendBlock(s string) {
	s = cleanTranscript(s)
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return
	}
	for _, line := range strings.Split(s, "\n") {
		a.transcript = append(a.transcript, line)
	}
	const maxTranscriptLines = 5000
	if len(a.transcript) > maxTranscriptLines {
		a.transcript = a.transcript[len(a.transcript)-maxTranscriptLines:]
	}
	a.scroll = 0
}

func cleanTranscript(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = stripANSI(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r >= 0x20 && r != 0x7f:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			continue
		}
		i++
		if s[i] == '[' {
			for i+1 < len(s) {
				i++
				if s[i] >= 0x40 && s[i] <= 0x7e {
					break
				}
			}
			continue
		}
		if s[i] == ']' {
			for i+1 < len(s) {
				i++
				if s[i] == 0x07 {
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i++
					break
				}
			}
			continue
		}
	}
	return b.String()
}

func (a *tuiApp) render() {
	a.width, a.height = terminalSize()
	w, h := a.width, a.height

	var b strings.Builder
	b.WriteString("\033[?25l\033[H\033[2J")

	if w < tuiMinWidth || h < tuiMinHeight {
		tuiMove(&b, 1, 1)
		b.WriteString("CHEW needs a little more terminal room.")
		tuiMove(&b, 2, 1)
		b.WriteString(fmt.Sprintf("Current: %dx%d  Needed: %dx%d", w, h, tuiMinWidth, tuiMinHeight))
		b.WriteString("\033[?25h")
		fmt.Fprint(os.Stdout, b.String())
		return
	}

	a.renderHeader(&b, w)
	a.renderTranscript(&b, w, h)
	a.renderPrompt(&b, w, h)

	fmt.Fprint(os.Stdout, b.String())
}

func (a *tuiApp) renderHeader(b *strings.Builder, width int) {
	opts := chewsprite.RenderOptions{
		TransparentBg: &[3]uint8{24, 24, 24},
	}
	frame := chewsprite.RenderFullCellByIndex(a.state.nextChewFrameIdx(), opts)
	if a.gumBlipActive() {
		frame = chewsprite.RenderGumFullCellByIndex(a.state.nextGumFrameIdx(), opts)
	}
	rows := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	side := a.headerSideText(width)
	for row := 1; row <= tuiMascotRows; row++ {
		tuiMove(b, row, 1)
		if row-1 < len(rows) {
			b.WriteString(rows[row-1])
		}
		if width >= 54 && row-1 < len(side) && side[row-1] != "" {
			tuiMove(b, row, 36)
			b.WriteString("\033[2m")
			b.WriteString(fitLine(side[row-1], width-35))
			b.WriteString("\033[0m")
		}
		b.WriteString("\033[K")
	}
	tuiMove(b, tuiHeaderRows, 1)
	b.WriteString("\033[2m")
	b.WriteString(fitLine(a.statusLine(width), width))
	b.WriteString("\033[0m\033[K")
}

func (a *tuiApp) headerSideText(width int) []string {
	projectName := "none"
	if pj := a.proj.Load(); pj != nil {
		projectName = pj.Name
	}
	brain := "not installed"
	if a.brain.Load() != nil {
		brain = "awake"
	} else if a.brainInstalled {
		brain = "napping"
	}
	lines := []string{
		"CHEW",
		"",
		"brain: " + brain,
		"project: " + projectName,
		"gum: " + a.gumLabel(),
	}
	lines = append(lines, a.work.SideLines()...)
	lines = append(lines,
		"",
		"PageUp/PageDown scroll",
		"Up/Down scroll one line",
		"Ctrl+U clears input",
		"Ctrl+C quits",
		"",
	)
	return lines
}

func (a *tuiApp) gumLabel() string {
	if a.gumKeyActive {
		return a.gumKey.Label()
	}
	if a.gumKeyErr != "" {
		return "key error"
	}
	return "public"
}

func (a *tuiApp) statusLine(width int) string {
	total := len(a.wrappedTranscript(width))
	visible := a.viewportHeight()
	position := "bottom"
	if a.scroll > 0 {
		position = fmt.Sprintf("%d lines up", a.scroll)
	}
	if total <= visible {
		position = "all text visible"
	}
	if a.brainBusy {
		return "dialog: " + position + " | brain busy; ask 'what are you doing?' for Gum status"
	}
	if a.gumStatusBusy {
		return "dialog: " + position + " | Gum status provider is checking orientation"
	}
	return "dialog: " + position + " | resize the terminal to show more"
}

func (a *tuiApp) renderTranscript(b *strings.Builder, width, height int) {
	lines := a.wrappedTranscript(width)
	viewHeight := a.viewportHeight()
	start := len(lines) - viewHeight - a.scroll
	if start < 0 {
		start = 0
	}
	end := start + viewHeight
	if end > len(lines) {
		end = len(lines)
	}

	for row := 0; row < viewHeight; row++ {
		tuiMove(b, tuiHeaderRows+1+row, 1)
		idx := start + row
		if idx < end {
			b.WriteString(fitLine(lines[idx], width))
		}
		b.WriteString("\033[K")
	}
}

func (a *tuiApp) renderPrompt(b *strings.Builder, width, height int) {
	line, cursorCol := promptLine(string(a.input), width)
	tuiMove(b, height, 1)
	b.WriteString("\033[1m")
	b.WriteString(line)
	b.WriteString("\033[0m\033[K")
	tuiMove(b, height, cursorCol)
	b.WriteString("\033[?25h")
}

func (a *tuiApp) wrappedTranscript(width int) []string {
	if width < 1 {
		width = 1
	}
	lines := make([]string, 0, len(a.transcript))
	for _, line := range a.transcript {
		lines = append(lines, wrapLine(line, width)...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func (a *tuiApp) viewportHeight() int {
	h := a.height - tuiHeaderRows - 1
	if h < 1 {
		return 1
	}
	return h
}

func (a *tuiApp) scrollBy(delta int) {
	total := len(a.wrappedTranscript(a.width))
	maxScroll := total - a.viewportHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	a.scroll += delta
	if a.scroll < 0 {
		a.scroll = 0
	}
	if a.scroll > maxScroll {
		a.scroll = maxScroll
	}
}

func (a *tuiApp) scrollToTop() {
	total := len(a.wrappedTranscript(a.width))
	a.scroll = max(0, total-a.viewportHeight())
}

func wrapLine(line string, width int) []string {
	line = strings.ReplaceAll(line, "\t", "    ")
	if width < 1 {
		width = 1
	}
	runes := []rune(line)
	if len(runes) == 0 {
		return []string{""}
	}
	var out []string
	for len(runes) > width {
		cut := width
		for i := width; i > 0; i-- {
			if unicode.IsSpace(runes[i-1]) {
				cut = i - 1
				break
			}
		}
		if cut <= 0 {
			cut = width
		}
		out = append(out, strings.TrimRightFunc(string(runes[:cut]), unicode.IsSpace))
		runes = trimLeadingSpace(runes[cut:])
	}
	out = append(out, string(runes))
	return out
}

func trimLeadingSpace(r []rune) []rune {
	for len(r) > 0 && unicode.IsSpace(r[0]) {
		r = r[1:]
	}
	return r
}

func fitLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 3 {
		return string(r[:width])
	}
	return string(r[:width-3]) + "..."
}

func promptLine(input string, width int) (string, int) {
	if width <= 2 {
		return ">", 2
	}
	prefix := "> "
	maxInput := width - len(prefix)
	r := []rune(input)
	if len(r) > maxInput {
		r = r[len(r)-maxInput:]
	}
	line := prefix + string(r)
	cursor := len([]rune(line)) + 1
	if cursor > width {
		cursor = width
	}
	return line, cursor
}

func tuiMove(b *strings.Builder, row, col int) {
	fmt.Fprintf(b, "\033[%d;%dH", row, col)
}
