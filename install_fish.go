package main

import (
	"github.com/adrg/xdg"
	"github.com/nsf/changedir/install"
)

const fishCD = `
function cd --wraps cd --description "cd wrapper which records directories history"
    builtin cd $argv
    if status is-interactive
        changedir put $PWD
    end
end
`

const fishCDInteractive = `
function cd-interactive --description "go to directory based on history (interactive)"
    # clear the line and move cursor to the beginning of the line (less flickering in some terminals)
    echo -ne "\033[2K\r"
    set -l destdir (changedir list | fzf --scheme=path --reverse --no-sort --no-info)
    if test $status -eq 0
        cd $destdir
    end
    commandline -f repaint
end
`

const fishConfig = `
if status is-interactive
    bind \cl cd-interactive
end
`

func installFish() {
	files := []install.File{
		{Action: install.Action_Write, Path: fatalr(xdg.ConfigFile("fish/functions/cd.fish")), Content: fishCD},
		{Action: install.Action_Write, Path: fatalr(xdg.ConfigFile("fish/functions/cd-interactive.fish")), Content: fishCDInteractive},
		{Action: install.Action_Append, Path: fatalr(xdg.ConfigFile("fish/config.fish")), Content: fishConfig},
	}
	doInstall(files)
}
