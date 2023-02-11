package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/adrg/xdg"
	"github.com/mitchellh/go-wordwrap"
	"go.etcd.io/bbolt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

func ww(s string) string {
	return wordwrap.WrapString(s, 80)
}

var BUCKET_DIRECTORIES = []byte("directories")
var BUCKET_IGNORES = []byte("ignores")

var ALL_BUCKETS = [][]byte{
	BUCKET_DIRECTORIES,
	BUCKET_IGNORES,
}

type DirectoryEntry struct {
	Path       []byte
	AccessTime []byte
}

type IgnoreEntry struct {
	RegExp []byte
}

type IgnoreEntryCompiled struct {
	RegExp *regexp.Regexp
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func fatalr[T any](v T, err error) T {
	fatal(err)
	return v
}

func closeDB(db **bbolt.DB) {
	if *db != nil {
		(*db).Close()
	}
}

func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func btoi(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

func getDBPath() string {
	return fatalr(xdg.DataFile("changedir/history.db"))
}

func loadDB() *bbolt.DB {
	db := fatalr(bbolt.Open(getDBPath(), 0600, nil))
	defer closeDB(&db)

	fatal(db.Update(func(tx *bbolt.Tx) error {
		for _, b := range ALL_BUCKETS {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	}))

	out := db
	db = nil
	return out
}

func commandList(db *bbolt.DB, args []string) {
	cmd := flag.NewFlagSet("changedir list", flag.ExitOnError)
	timestamps := cmd.Bool("time", false, "add timestamps to output (tab separated)")
	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "Usage: changedir list [options]\n")
		fmt.Fprintf(cmd.Output(), ww("\nList all directories in most recently stored first order.\n"))
		fmt.Fprintf(cmd.Output(), "\nOptions:\n")
		cmd.PrintDefaults()
	}
	cmd.Parse(args)

	var out []DirectoryEntry
	fatal(db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BUCKET_DIRECTORIES)
		n := b.Stats().KeyN
		out = make([]DirectoryEntry, 0, n)
		return b.ForEach(func(k, v []byte) error {
			out = append(out, DirectoryEntry{
				Path:       k,
				AccessTime: v,
			})
			return nil
		})
	}))
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return bytes.Compare(a.AccessTime, b.AccessTime) == 1
	})
	w := bufio.NewWriter(os.Stdout)
	for _, e := range out {
		if *timestamps {
			fatalr(w.Write(e.AccessTime))
			fatal(w.WriteByte('\t'))
		}
		fatalr(w.Write(e.Path))
		fatal(w.WriteByte('\n'))
	}
	fatal(w.Flush())
}

func commandPut(db *bbolt.DB, args []string) {
	cmd := flag.NewFlagSet("changedir put", flag.ExitOnError)
	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "Usage: changedir put [directory]\n")
		fmt.Fprintf(cmd.Output(), ww("\nPut a directory to history unless it passes a check from ignore list. If directory is empty or argument is missing, the command silently does nothing.\n"))
		cmd.PrintDefaults()
	}
	cmd.Parse(args)

	now := []byte(time.Now().UTC().Format(time.RFC3339))
	dirString := strings.TrimSpace(cmd.Arg(0))
	if dirString == "" {
		// do nothing
		return
	}
	dir := []byte(dirString)

	regexps := compileIgnoreList(getIgnoreList(db))
	for _, r := range regexps {
		if r.RegExp.Match(dir) {
			// this entry must be ignored
			return
		}
	}

	fatal(db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BUCKET_DIRECTORIES)
		return b.Put(dir, now)
	}))
}

func commandRemove(db *bbolt.DB, args []string) {
	cmd := flag.NewFlagSet("changedir remove", flag.ExitOnError)
	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "Usage: changedir remove [directory]\n")
		fmt.Fprintf(cmd.Output(), ww("\nRemove a directory from history. If directory is empty or argument is missing, the command silently does nothing.\n"))
		cmd.PrintDefaults()
	}
	cmd.Parse(args)

	dirString := cmd.Arg(0)
	if dirString == "" {
		// do nothing
		return
	}
	dir := []byte(dirString)
	fatal(db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BUCKET_DIRECTORIES)
		return b.Delete(dir)
	}))
}

func commandInstall(db *bbolt.DB, args []string) {
	cmd := flag.NewFlagSet("changedir install", flag.ExitOnError)
	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "Usage: changedir install\n")
		fmt.Fprintf(cmd.Output(), ww("\nInstall shell integration. This command is interactive. Before writing anything to any file it will print the details and ask for confirmation. Don't hesitate to run it and see if what it does suits your needs.\n"))
		cmd.PrintDefaults()
	}
	cmd.Parse(args)

	fmt.Printf("Installation is available for the following shells:\n")
	fmt.Printf("1) fish\n")
	n := fatalr(askInt("Which shell do you want to install files for? ", 0))
	if n != 1 {
		fatal(fmt.Errorf("Please, pick 1"))
	}

	switch n {
	case 1:
		installFish()
	}
}

func commandPrune(db *bbolt.DB, args []string) {
	cmd := flag.NewFlagSet("changedir prune", flag.ExitOnError)
	dry := cmd.Bool("dry", false, "only print the results without actually removing anything")
	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "Usage: changedir prune [options]\n")
		fmt.Fprintf(cmd.Output(), ww("\nRemove non-existent directories from history. Also removes entries which are not a directory.\n"))
		fmt.Fprintf(cmd.Output(), "\nOptions:\n")
		cmd.PrintDefaults()
	}
	cmd.Parse(args)

	var toRemove [][]byte
	w := bufio.NewWriter(os.Stdout)
	fatal(db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BUCKET_DIRECTORIES)
		return b.ForEach(func(k, v []byte) error {
			fi, err := os.Stat(string(k))
			isNotExist := os.IsNotExist(err)
			notDir := err == nil && !fi.IsDir()
			if isNotExist || notDir {
				toRemove = append(toRemove, k)
				if isNotExist {
					fatalr(w.WriteString("[MISSING] "))
				} else {
					fatalr(w.WriteString("[NOTADIR] "))
				}
				fatalr(w.Write(k))
				fatal(w.WriteByte('\n'))
			}
			return nil
		})
	}))
	fatal(w.Flush())

	if !*dry {
		fatal(db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket(BUCKET_DIRECTORIES)
			for _, k := range toRemove {
				if err := b.Delete(k); err != nil {
					return err
				}
			}
			return nil
		}))
	}
}

func getIgnoreList(db *bbolt.DB) []IgnoreEntry {
	var out []IgnoreEntry
	fatal(db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BUCKET_IGNORES)
		out = make([]IgnoreEntry, 0, b.Stats().KeyN)
		return b.ForEach(func(k, v []byte) error {
			out = append(out, IgnoreEntry{RegExp: k})
			return nil
		})
	}))
	return out
}

func compileIgnoreList(list []IgnoreEntry) []IgnoreEntryCompiled {
	out := make([]IgnoreEntryCompiled, 0, len(list))
	for _, v := range list {
		r, err := regexp.Compile(string(v.RegExp))
		if err == nil {
			out = append(out, IgnoreEntryCompiled{RegExp: r})
		}
	}
	return out
}

func commandIgnoreList(db *bbolt.DB, args []string) {
	cmd := flag.NewFlagSet("changedir ignore list", flag.ExitOnError)
	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "Usage: changedir ignore list\n")
		fmt.Fprintf(cmd.Output(), ww("\nList all regexps from ignore list. All regexps are enclosed in '' quotes, this is to help you see spaces in regexps, which are allowed.\n"))
		cmd.PrintDefaults()
	}
	cmd.Parse(args)

	regexps := getIgnoreList(db)
	w := bufio.NewWriter(os.Stdout)
	for _, r := range regexps {
		fatal(w.WriteByte('\''))
		fatalr(w.Write(r.RegExp))
		fatal(w.WriteByte('\''))
		fatal(w.WriteByte('\n'))
	}
	fatal(w.Flush())
}

func commandIgnorePut(db *bbolt.DB, args []string) {
	cmd := flag.NewFlagSet("changedir ignore put", flag.ExitOnError)
	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "Usage: changedir ignore put <regexp>\n")
		fmt.Fprintf(cmd.Output(), ww("\nAdd a regexp to the list. Matching directories will not be stored.\n"))
		cmd.PrintDefaults()
	}
	cmd.Parse(args)

	regexpString := cmd.Arg(0)
	if regexpString == "" {
		cmd.Usage()
		return
	}

	regexp := []byte(regexpString)
	fatal(db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BUCKET_IGNORES)
		return b.Put(regexp, nil)
	}))
}

func commandIgnoreRemove(db *bbolt.DB, args []string) {
	cmd := flag.NewFlagSet("changedir ignore remove", flag.ExitOnError)
	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "Usage: changedir ignore remove <regexp>\n")
		fmt.Fprintf(cmd.Output(), ww("\nRemove a regexp from the list.\n"))
		cmd.PrintDefaults()
	}
	cmd.Parse(args)

	regexpString := cmd.Arg(0)
	if regexpString == "" {
		// do nothing
		return
	}
	regexp := []byte(regexpString)

	fatal(db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BUCKET_IGNORES)
		return b.Delete(regexp)
	}))
}

func commandIgnoreApply(db *bbolt.DB, args []string) {
	cmd := flag.NewFlagSet("changedir ignore apply", flag.ExitOnError)
	dry := cmd.Bool("dry", false, "only print the results without actually removing anything")
	cmd.Usage = func() {
		fmt.Fprintf(cmd.Output(), "Usage: changedir ignore apply [options]\n")
		fmt.Fprintf(cmd.Output(), ww("\nApply ignore list to existing entries. The command will print out deleted directories.\n"))
		fmt.Fprintf(cmd.Output(), "\nOptions:\n")
		cmd.PrintDefaults()
	}
	cmd.Parse(args)

	_ = dry

	var toRemove [][]byte

	regexps := compileIgnoreList(getIgnoreList(db))
	w := bufio.NewWriter(os.Stdout)
	fatal(db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(BUCKET_DIRECTORIES)
		return b.ForEach(func(k, v []byte) error {
			matches := false
			for _, r := range regexps {
				if r.RegExp.Match(k) {
					matches = true
					break
				}
			}
			if matches {
				toRemove = append(toRemove, k)
				fatalr(w.Write(k))
				fatal(w.WriteByte('\n'))
			}
			return nil
		})
	}))
	fatal(w.Flush())

	if !*dry {
		fatal(db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket(BUCKET_DIRECTORIES)
			for _, k := range toRemove {
				if err := b.Delete(k); err != nil {
					return err
				}
			}
			return nil
		}))
	}
}

func getSubCommand(args []string) (string, []string) {
	subCommand := ""
	if len(args) > 0 {
		subCommand = args[0]
		args = args[1:]
	}
	return subCommand, args
}

func main() {
	cmd := flag.NewFlagSet("changedir", flag.ExitOnError)
	cmd.Usage = func() {
		o := cmd.Output()
		fmt.Fprintf(o, "Usage: changedir [command] [args]\n")
		fmt.Fprintf(o, ww("\nUtility that helps you maintain visited directories history. You can get help for any command using -h flag, e.g.: `changedir ignore apply -h`.\n"))
		fmt.Fprintf(o, "\nAvailable commands:\n")
		fmt.Fprintf(o, "  list             list all directories\n")
		fmt.Fprintf(o, "  put              put a directory to history\n")
		fmt.Fprintf(o, "  remove           remove a directory from history\n")
		fmt.Fprintf(o, "  prune            remove non-existent directories from history\n")
		fmt.Fprintf(o, "  ignore list      list all regexps from ignore list\n")
		fmt.Fprintf(o, "  ignore put       put a regexp to ignore list\n")
		fmt.Fprintf(o, "  ignore remove    remove a regexp from ignore list\n")
		fmt.Fprintf(o, "  ignore apply     apply ignore list to existing entries\n")
		fmt.Fprintf(o, "  install          install shell integration (interactive)\n")
		fmt.Fprintf(o, "\nDatabase location:\n")
		fmt.Fprintf(o, "  %s\n", getDBPath())
		cmd.PrintDefaults()
	}
	cmd.Parse(os.Args[1:])

	db := loadDB()
	defer db.Close()

	subCommand, args := getSubCommand(cmd.Args())
	switch subCommand {
	default:
		fallthrough
	case "":
		cmd.Usage()
	case "list":
		commandList(db, args)
	case "put":
		commandPut(db, args)
	case "remove":
		commandRemove(db, args)
	case "prune":
		commandPrune(db, args)
	case "install":
		commandInstall(db, args)
	case "ignore":
		subCommand, args := getSubCommand(args)
		switch subCommand {
		default:
			cmd.Usage()
		case "list":
			commandIgnoreList(db, args)
		case "put":
			commandIgnorePut(db, args)
		case "remove":
			commandIgnoreRemove(db, args)
		case "apply":
			commandIgnoreApply(db, args)
		}
	}
}
