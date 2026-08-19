// FluxVCL 7GUIs CRUD：StringGrid 行选择、稳定业务 ID、筛选与增删改。
//
// 设计边界见 docs/design.md §21.2、§21.4 与 docs/7guis.md。人员模型、
// 筛选规则和稳定 ID 全部留在示例层，StringGrid 只接收受控字符串矩阵。
//
// 构建：scripts/build.ps1 -Target 7guis-crud
// 冒烟：scripts/smoke.ps1 -Target 7guis-crud
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/native"
)

type person struct {
	ID      int
	Name    string
	Surname string
}

type directoryState struct {
	People     []person
	SelectedID int
	NextID     int
	Message    string
}

func (s directoryState) String() string {
	selection := "none"
	if s.SelectedID != 0 {
		selection = strconv.Itoa(s.SelectedID)
	}
	return fmt.Sprintf("Records: %d   Selected ID: %s   %s", len(s.People), selection, s.Message)
}

func (s directoryState) clone() directoryState {
	s.People = append([]person(nil), s.People...)
	return s
}

func filteredPeople(people []person, filter string) []person {
	prefix := strings.ToLower(strings.TrimSpace(filter))
	if prefix == "" {
		return append([]person(nil), people...)
	}
	result := make([]person, 0, len(people))
	for _, entry := range people {
		if strings.HasPrefix(strings.ToLower(entry.Surname), prefix) {
			result = append(result, entry)
		}
	}
	return result
}

func personIndexByID(people []person, id int) int {
	for i := range people {
		if people[i].ID == id {
			return i
		}
	}
	return -1
}

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法获取 exe 路径:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target 7guis-crud 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)
	filter := flux.NewState("")
	name := flux.NewState("Hans")
	surname := flux.NewState("Emil")
	directory := flux.NewState(directoryState{
		People: []person{
			{ID: 1001, Name: "Hans", Surname: "Emil"},
			{ID: 1002, Name: "Max", Surname: "Mustermann"},
			{ID: 1003, Name: "Roman", Surname: "Rex"},
		},
		SelectedID: 1001,
		NextID:     1004,
		Message:    "Ready",
	})

	loadEditor := func(entry *person) {
		if entry == nil {
			name.Set("")
			surname.Set("")
			return
		}
		name.Set(entry.Name)
		surname.Set(entry.Surname)
	}
	selectFirstVisible := func(next directoryState, filterText string) directoryState {
		visible := filteredPeople(next.People, filterText)
		if len(visible) == 0 {
			next.SelectedID = 0
			loadEditor(nil)
			return next
		}
		next.SelectedID = visible[0].ID
		selected := visible[0]
		loadEditor(&selected)
		return next
	}

	if err := app.Mount(func() flux.Widget {
		snapshot := directory.Get()
		visible := filteredPeople(snapshot.People, filter.Get())
		cells := make([][]string, len(visible))
		selectedRow := -1
		for row, entry := range visible {
			cells[row] = []string{entry.Surname, entry.Name, strconv.Itoa(entry.ID)}
			if entry.ID == snapshot.SelectedID {
				selectedRow = row
			}
		}

		selection := flux.SelectedCell(-1, -1)
		selectedVisible := selectedRow >= 0
		if len(visible) > 0 {
			if selectedRow < 0 {
				selectedRow = 0
			}
			selection = flux.SelectedRow(selectedRow)
		}
		canCreate := strings.TrimSpace(name.Get()) != "" && strings.TrimSpace(surname.Get()) != ""

		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - 7GUIs CRUD"),
			flux.Column(
				flux.CrossAxis(flux.CrossAxisStretch),
				flux.Row(
					flux.Text("Filter prefix"),
					flux.Expanded(flux.Input(
						flux.Bind(filter),
						flux.Key("filter"),
						flux.OnChange(func(value string) {
							filter.Set(value)
							next := directory.Get().clone()
							filtered := filteredPeople(next.People, value)
							if personIndexByID(filtered, next.SelectedID) < 0 {
								next = selectFirstVisible(next, value)
								next.Message = "Filter changed"
								directory.Set(next)
							}
						}),
					)),
				),
				flux.Expanded(flux.StringGrid(len(visible), 3,
					flux.Key("people-grid"),
					flux.AccessibleName("People directory"),
					flux.AccessibleDescription("Use the arrow keys to select a person."),
					flux.Headers([]string{"Surname", "Name", "ID"}),
					flux.ColumnWidths([]int{150, 145, 64}),
					flux.Cells(cells),
					selection,
					flux.Editable(false),
					flux.OnCellSelect(func(cell flux.GridCell) {
						if cell.Row < 0 || cell.Row >= len(visible) {
							return
						}
						entry := visible[cell.Row]
						next := directory.Get().clone()
						next.SelectedID = entry.ID
						next.Message = "Selected stable ID " + strconv.Itoa(entry.ID)
						directory.Set(next)
						loadEditor(&entry)
					}),
				)),
				flux.Row(
					flux.Text("Name"),
					flux.Expanded(flux.Input(flux.Bind(name), flux.Key("name"))),
					flux.Text("Surname"),
					flux.Expanded(flux.Input(flux.Bind(surname), flux.Key("surname"))),
				),
				flux.Row(
					flux.Button("Create", flux.Enabled(canCreate), flux.OnClick(func(_ flux.Event) {
						newName := strings.TrimSpace(name.Get())
						newSurname := strings.TrimSpace(surname.Get())
						if newName == "" || newSurname == "" {
							return
						}
						filter.Set("")
						next := directory.Get().clone()
						id := next.NextID
						next.NextID++
						next.People = append(next.People, person{ID: id, Name: newName, Surname: newSurname})
						next.SelectedID = id
						next.Message = "Created stable ID " + strconv.Itoa(id)
						directory.Set(next)
						name.Set(newName)
						surname.Set(newSurname)
					})),
					flux.Button("Update", flux.Enabled(selectedVisible && canCreate), flux.OnClick(func(_ flux.Event) {
						next := directory.Get().clone()
						index := personIndexByID(next.People, next.SelectedID)
						if index < 0 {
							return
						}
						next.People[index].Name = strings.TrimSpace(name.Get())
						next.People[index].Surname = strings.TrimSpace(surname.Get())
						next.Message = "Updated stable ID " + strconv.Itoa(next.SelectedID)
						filter.Set("")
						directory.Set(next)
					})),
					flux.Button("Delete", flux.Enabled(selectedVisible), flux.OnClick(func(_ flux.Event) {
						next := directory.Get().clone()
						index := personIndexByID(next.People, next.SelectedID)
						if index < 0 {
							return
						}
						deletedID := next.SelectedID
						next.People = append(next.People[:index], next.People[index+1:]...)
						next = selectFirstVisible(next, filter.Get())
						next.Message = "Deleted stable ID " + strconv.Itoa(deletedID)
						directory.Set(next)
					})),
					flux.Expanded(flux.Text(flux.Bind(directory))),
				),
			),
		)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "挂载失败:", err)
		os.Exit(2)
	}

	native.Run()
}
