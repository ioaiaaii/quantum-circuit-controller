// Standalone visual demo of the qcc CLI render pipeline.
// Run with: go run ./hack/demo
package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/ioaiaaii/quantum-circuit-controller/internal/cli/render"
)

func main() {
	fmt.Print(render.Banner("0653eba-dirty"))
	fmt.Println()
	fmt.Print(render.Step("compiling bell.qasm", "… 14 ms"))
	fmt.Print(render.Step("-h --help                  help for qcc", ""))
	fmt.Print(render.Step("-v, --version               version for qcc", ""))
	fmt.Print(render.OK("targeting aer_simulator", ""))
	fmt.Print(render.OK("queued · job aer-7f2d8e1b", ""))
	fmt.Print(render.OK("completed · 1.34s", ""))
	fmt.Println()

	body := render.KV([][2]string{
		{"backend", "aer_simulator"},
		{"shots", "2024"},
		{"mode", "execute"},
		{"task id", "aer-7f2d8e1b"},
		{"duration", "1.34s"},
	})
	body += "\n" + render.Histogram(map[string]int64{
		"001":  200,
		"0101": 600,
	})
	fmt.Print(render.Section("results", body))

	var (
		purple    = lipgloss.Color("99")
		gray      = lipgloss.Color("245")
		lightGray = lipgloss.Color("241")

		headerStyle  = lipgloss.NewStyle().Foreground(purple).Bold(true).Align(lipgloss.Center)
		cellStyle    = lipgloss.NewStyle().Padding(0, 1).Width(14)
		oddRowStyle  = cellStyle.Foreground(gray)
		evenRowStyle = cellStyle.Foreground(lightGray)
	)

	rows := [][]string{
		{"Chinese", "您好", "你好"},
		{"Japanese", "こんにちは", "やあ"},
		{"Arabic", "أهلين", "أهلا"},
		{"Russian", "Здравствуйте", "Привет"},
		{"Spanish", "Hola", "¿Qué tal?"},
	}
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(purple)).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return headerStyle
			case row%2 == 0:
				return evenRowStyle
			default:
				return oddRowStyle
			}
		}).
		Headers("LANGUAGE", "FORMAL", "INFORMAL").
		Rows(rows...)

	// You can also add tables row-by-row
	t.Row("English", "You look absolutely fabulous.", "How's it going?")

}
