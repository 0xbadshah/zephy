package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// The wordmark matches assets/banner.svg: there each letter is a 5x7
// grid of pixel rects; here the same grid renders with block characters,
// doubled horizontally so the pixels come out square. If the SVG wordmark
// changes, these bitmaps change with it - the terminal and the banner
// stay twins.

var wordmarkLetters = [][]string{
	{ // z
		"█████",
		"....█",
		"...█.",
		"..█..",
		".█...",
		"█....",
		"█████",
	},
	{ // e
		".███.",
		"█...█",
		"█...█",
		"█████",
		"█....",
		"█...█",
		".███.",
	},
	{ // p
		"████.",
		"█...█",
		"█...█",
		"████.",
		"█....",
		"█....",
		"█....",
	},
	{ // h
		"█....",
		"█....",
		"█████",
		"█...█",
		"█...█",
		"█...█",
		"█...█",
	},
	{ // y
		"█...█",
		"█...█",
		"█...█",
		".████",
		"....█",
		"....█",
		"████.",
	},
}

// wordmarkStyle uses the banner's brand purple (the scan beam). The bottom
// margin puts one blank line between the wordmark and the scrollback.
var wordmarkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7c5cff")).MarginBottom(1)

// Banner renders the pixel wordmark: the terminal twin of the SVG
// banner. It leads the progress scrollback; plain and CI output never
// see it.
func Banner() string {
	var b strings.Builder
	for row := 0; row < 7; row++ {
		b.WriteString(" ") // match the log's one-space left rhythm
		for li, letter := range wordmarkLetters {
			if li > 0 {
				b.WriteString("  ")
			}
			for _, c := range letter[row] {
				if c == '█' {
					b.WriteString("██")
				} else {
					b.WriteString("  ")
				}
			}
		}
		b.WriteString("\n")
	}
	return wordmarkStyle.Render(strings.TrimRight(b.String(), "\n"))
}
