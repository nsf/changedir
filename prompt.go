package main

import (
	"bufio"
	"errors"
	"github.com/fatih/color"
	"golang.org/x/term"
	"os"
	"strconv"
	"strings"
)

func askByte(text string, hint string, def byte, onResponse func(v byte) error) (result byte, err error) {
	fd := int(os.Stdout.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return 0, err
	}

	defer func() {
		if lerr := term.Restore(fd, state); lerr != nil {
			err = lerr
			return
		}
		if lerr := onResponse(result); lerr != nil {
			err = lerr
			return
		}
	}()

	bold := color.New(color.Bold)

	if _, err := os.Stdout.WriteString(color.CyanString("▶ ") + bold.Sprint(text)); err != nil {
		return 0, err
	}
	if len(hint) > 0 {
		if _, err := os.Stdout.WriteString(bold.Sprint(hint)); err != nil {
			return 0, err
		}
	}
	b := make([]byte, 1)
	if _, err := os.Stdin.Read(b); err != nil {
		return 0, err
	}
	if b[0] == '\n' || b[0] == '\r' {
		return def, nil
	}
	return b[0], nil
}

var ErrInvalidYesNoResponse = errors.New("Invalid response, please, use 'y' or 'n'")

func askYesNo(text string, def bool) (result bool, err error) {
	defByte := byte('y')
	hint := " [Y/n] "
	if !def {
		defByte = byte('n')
		hint = " [y/N] "
	}
	b, err := askByte(text, hint, defByte, func(v byte) error {
		var err error
		switch v {
		case 'y', 'Y':
			_, err = os.Stdout.WriteString("Yes\n")
		case 'n', 'N':
			_, err = os.Stdout.WriteString("No\n")
		default:
			err = ErrInvalidYesNoResponse
		}
		return err
	})
	if err != nil {
		return false, err
	}
	switch b {
	case 'y', 'Y':
		return true, nil
	case 'n', 'N':
		return false, nil
	}
	panic("unreachable")
}

func askInt(text string, def int) (int, error) {
	if _, err := os.Stdout.WriteString(color.CyanString("▶ ") + color.New(color.Bold).Sprint(text)); err != nil {
		return 0, err
	}
	r := bufio.NewReader(os.Stdin)
	response, err := r.ReadString('\n')
	if err != nil {
		return 0, err
	}
	response = strings.TrimSpace(response)
	if response == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(response, 10, 64)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
