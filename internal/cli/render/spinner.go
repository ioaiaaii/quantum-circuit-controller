/*
Copyright 2026 ioaiaaii.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package render

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// braille is the visual idiom Charm uses across most of its CLIs.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 100 * time.Millisecond

// Spinner is a single-line animated indicator that overwrites itself in place.
// Outside a TTY (pipes, CI logs) it degrades to plain "▸ message" lines so the
// output stays grep-friendly.
type Spinner struct {
	w     io.Writer
	isTTY bool

	mu      sync.Mutex
	text    string
	idx     int
	running bool
	done    chan struct{}
}

// NewSpinner returns a spinner that writes to w.  If w is a *os.File and
// connected to a terminal, animation is enabled; otherwise the spinner is a
// no-op that prints plain step lines on Finish.
func NewSpinner(w io.Writer) *Spinner {
	return &Spinner{w: w, isTTY: isTerminal(w)}
}

// Start begins the animation with the given message.  Idempotent: calling
// Start twice without Finish in between just updates the text.
func (s *Spinner) Start(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.text = text
	if !s.isTTY {
		_, _ = fmt.Fprintf(s.w, "%s %s\n", stepStyle.Render("▸"), text)
		return
	}
	if s.running {
		s.draw()
		return
	}
	s.running = true
	s.done = make(chan struct{})
	go s.tick(s.done)
}

// Update changes the spinner's text without advancing the animation phase.
func (s *Spinner) Update(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.text = text
	if s.isTTY && s.running {
		s.draw()
	}
}

// FinishOK stops the spinner and prints a final line prefixed by ✓.
func (s *Spinner) FinishOK(text string) {
	s.finish(okStyle.Render("✓"), text)
}

// FinishFail stops the spinner and prints a final line prefixed by ✗.
func (s *Spinner) FinishFail(text string) {
	s.finish(failStyle.Render("✗"), text)
}

// FinishStep stops the spinner and prints a final line prefixed by ▸ (use
// when a phase has completed but isn't a hard success — e.g., handed off to a
// further async step).
func (s *Spinner) FinishStep(text string) {
	s.finish(stepStyle.Render("▸"), text)
}

// Stop cancels the animation and clears the line without writing anything.
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isTTY && s.running {
		close(s.done)
		s.running = false
		_, _ = fmt.Fprint(s.w, "\r\x1b[2K")
	}
}

func (s *Spinner) finish(icon, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isTTY && s.running {
		close(s.done)
		s.running = false
		_, _ = fmt.Fprintf(s.w, "\r\x1b[2K%s %s\n", icon, text)
		return
	}
	_, _ = fmt.Fprintf(s.w, "%s %s\n", icon, text)
}

func (s *Spinner) tick(done chan struct{}) {
	t := time.NewTicker(spinnerInterval)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			s.mu.Lock()
			s.idx++
			s.draw()
			s.mu.Unlock()
		}
	}
}

func (s *Spinner) draw() {
	frame := spinnerFrames[s.idx%len(spinnerFrames)]
	_, _ = fmt.Fprintf(s.w, "\r\x1b[2K%s %s", stepStyle.Render(frame), s.text)
}

// isTerminal reports whether w is an interactive terminal.  We avoid pulling
// in golang.org/x/term by relying on os.File.Stat's character-device bit,
// which is enough for our pipes-vs-tty distinction.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
