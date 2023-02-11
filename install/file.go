package install

import (
	"bytes"
	"fmt"
	"github.com/fatih/color"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Action int

const (
	Action_Write Action = iota
	Action_Append
)

type File struct {
	Exists      bool
	Installed   bool
	Action      Action
	Path        string
	Content     string
	ContentData []byte
}

func (f *File) Summary() string {
	qpath := fmt.Sprintf(" %q", f.Path)
	if f.Installed {
		return color.GreenString("Already installed:") + qpath
	}
	if f.Action == Action_Append {
		return color.YellowString("Append to:") + qpath
	}
	if f.Exists {
		return color.RedString("Overwrite:") + qpath
	} else {
		return color.GreenString("Create:") + qpath
	}
}

func (f *File) SummaryAction() string {
	if f.Installed {
		return fmt.Sprintf("File %q is already installed", f.Path)
	}
	if f.Action == Action_Append {
		return fmt.Sprintf("Appending data to file %q", f.Path)
	}
	if f.Exists {
		return fmt.Sprintf("Overwriting the file %q", f.Path)
	} else {
		return fmt.Sprintf("Creating the file %q", f.Path)
	}
}

func (f *File) Apply() error {
	if f.Installed {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(f.Path), 0644); err != nil {
		return err
	}

	switch f.Action {
	case Action_Append:
		if f.Exists {
			file, err := os.OpenFile(f.Path, os.O_APPEND|os.O_RDWR, 0644)
			if err != nil {
				return err
			}
			defer file.Close()
			if _, err := file.WriteString("\n"); err != nil {
				return err
			}
			if _, err := file.Write(f.ContentData); err != nil {
				return err
			}
		} else {
			if err := os.WriteFile(f.Path, f.ContentData, 0644); err != nil {
				return err
			}
		}
	case Action_Write:
		if err := os.WriteFile(f.Path, f.ContentData, 0644); err != nil {
			return err
		}
	}
	return nil
}

func withoutColor(cb func()) {
	saveColor := color.NoColor
	color.NoColor = true
	cb()
	color.NoColor = saveColor
}

func (f *File) maxLen() int {
	maxLen := 0
	withoutColor(func() {
		if n := utf8.RuneCountInString(f.Summary()); n > maxLen {
			maxLen = n
		}
		for _, line := range strings.Split(f.Content, "\n") {
			if n := utf8.RuneCountInString(line); n > maxLen {
				maxLen = n
			}
		}
	})
	return maxLen
}

func padString(f func() string, n int) string {
	var s string
	withoutColor(func() {
		s = f()
	})

	var b strings.Builder
	b.WriteString(f())
	for i := utf8.RuneCountInString(s); i < n; i++ {
		b.WriteByte(' ')
	}
	return b.String()
}

func sep(s string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(s)
	}
	return b.String()
}

func (f *File) printDetails(maxLen int) {
	fmt.Printf("║ %s ║\n", padString(f.Summary, maxLen))
	fmt.Printf("╟%s╢\n", sep("┄", maxLen+2))

	lines := strings.Split(f.Content, "\n")
	for _, line := range lines {
		f := func() string { return line }
		fmt.Printf("║ %s ║\n", padString(f, maxLen))
	}
}

func PrintDetails(files []File) {
	maxLen := 0
	for _, f := range files {
		if n := f.maxLen(); n > maxLen {
			maxLen = n
		}
	}

	fmt.Printf("╔%s╗\n", sep("═", maxLen+2))
	for i, f := range files {
		if i != 0 {
			fmt.Printf("╠%s╣\n", sep("═", maxLen+2))
		}
		f.printDetails(maxLen)
	}
	fmt.Printf("╚%s╝\n", sep("═", maxLen+2))
}

func Apply(files []File) error {
	for _, f := range files {
		fmt.Println(f.SummaryAction())
		if err := f.Apply(); err != nil {
			return err
		}
	}
	return nil
}

func IsInstalled(files []File) bool {
	for _, f := range files {
		if !f.Installed {
			return false
		}
	}
	return true
}

func Prepare(files []File) error {
	for i := range files {
		file := &files[i]

		file.ContentData = []byte(strings.TrimSpace(file.Content) + "\n")

		_, err := os.Stat(file.Path)
		file.Exists = true
		if err != nil {
			if os.IsNotExist(err) {
				file.Exists = false
			} else {
				return err
			}
		}

		if file.Exists {
			data, err := os.ReadFile(file.Path)
			if err != nil {
				return err
			}
			if file.Action == Action_Write {
				file.Installed = bytes.Equal(data, file.ContentData)
			} else if file.Action == Action_Append {
				file.Installed = bytes.Contains(data, file.ContentData)
			}
		}

		file.Content = strings.TrimSpace(file.Content)
	}
	return nil
}
