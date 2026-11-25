package tui

import (
	"fmt"
	"io"
)

func hobnobLead(mark string) string {
	return SBold.Render("hobnob") + " " + mark + " "
}

// HobnobError writes hobnob's own error, which takes a task's place in the
// log: "hobnob ✗ msg".
func HobnobError(w io.Writer, msg string) {
	fmt.Fprintln(w, hobnobLead(SError.Render("✗"))+msg)
}

// HobnobWarning writes a warning after which hobnob carries on: "hobnob ! msg".
func HobnobWarning(w io.Writer, msg string) {
	fmt.Fprintln(w, hobnobLead(SStep.Render("!"))+msg)
}

// HobnobNote writes a grey note: "hobnob » msg".
func HobnobNote(w io.Writer, msg string) {
	fmt.Fprintln(w, hobnobLead(SInfo.Render("»"))+SInfo.Render(msg))
}

// HobnobDone writes hobnob's own success: "hobnob ✓ msg".
func HobnobDone(w io.Writer, msg string) {
	fmt.Fprintln(w, hobnobLead(SChecked.Render("✓"))+msg)
}

// HobnobOutput returns a writer that prefixes each line it is given with
// "hobnob │ ", for the output of a program hobnob runs itself.
func HobnobOutput(w io.Writer) *LineWriter {
	return NewLineWriter(w, hobnobLead(SInfo.Render("│")))
}
