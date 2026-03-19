package tui

import (
	"charm.land/lipgloss/v2"
)

var (
	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))

	statusRunning = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		SetString("● running")

	statusIdle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		SetString("○ idle")

	statusActive = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD700"))

	statusCreating = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		SetString("◌ creating...")

	statusStopping = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		SetString("◌ stopping...")

	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000"))

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262"))

	filterPromptStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true)

	spinnerStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4"))

	mutagenOK = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575"))

	mutagenErr = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000"))

	previewBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(0, 1)

	previewHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4"))

	// Init wizard styles
	stepDoneStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575"))

	stepErrStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000"))

	stepActiveStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4"))

	stepPendingStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))

	checkOn = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		SetString("[x]")

	checkOff = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		SetString("[ ]")

	statusTeam = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00BFFF"))

	teamMemberActive = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD700"))

	teamMemberIdle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))

	taskDone = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575"))

	taskInProgress = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD700"))

	taskPending = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))
)
