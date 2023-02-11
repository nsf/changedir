package main

import (
	"errors"
	"fmt"
	"github.com/nsf/changedir/install"
)

func doInstall(files []install.File) {
	fatal(install.Prepare(files))

	// Initial "Apply actions?"
	fmt.Println("────────────────")
	if install.IsInstalled(files) {
		fmt.Println("Already installed!")
		return
	}
	fmt.Println("The following actions will be performed:")
	for _, f := range files {
		fmt.Println(f.Summary())
	}
	b := fatalr(askByte("Apply the actions? ('?' for details)", " [y/N/?] ", 'n', func(v byte) error {
		var err error
		switch v {
		case 'y', 'Y':
			_, err = fmt.Println("Yes")
		case 'n', 'N':
			_, err = fmt.Println("No")
		case '?':
			_, err = fmt.Println("Details")
		default:
			err = errors.New("Invalid response, please, use 'y', 'n' or '?'")
		}
		return err
	}))

	switch b {
	case 'y', 'Y':
		// Just apply all actions
		fmt.Println("────────────────")
		fatal(install.Apply(files))
	case 'n', 'N':
		// No, do nothing
	case '?':
		// Show details
		fmt.Println("────────────────")
		install.PrintDetails(files)
		b := fatalr(askByte("Apply the actions? ('s' for step by step)", " [y/N/s] ", 'n', func(v byte) error {
			var err error
			switch v {
			case 'y', 'Y':
				_, err = fmt.Println("Yes")
			case 'n', 'N':
				_, err = fmt.Println("No")
			case 's', 'S':
				_, err = fmt.Println("Step by step")
			default:
				err = errors.New("Invalid response, please, use 'y', 'n' or 's'")
			}
			return err
		}))

		switch b {
		case 'y', 'Y':
			// Just apply all actions
			fmt.Println("────────────────")
			fatal(install.Apply(files))
		case 'n', 'N':
		// No, do nothing
		case 's', 'S':
			// Apply actions step by step
			for _, f := range files {
				fmt.Println("────────────────")
				lfiles := []install.File{f}
				install.PrintDetails(lfiles)
				if fatalr(askYesNo("Apply the action?", false)) {
					fatal(install.Apply(lfiles))
				}
			}
		}
	}
}
