// asm - Anvil Session Manager
//
// This tool allows you to:
//
//  1. Dump all of your running Anvil instances to one directory
//  2. Later, from the command line, start an Anvil instance for each dump in a directory and load the saved dumpfile
//
// This is useful if you want to close all your anvil sessions and then reload them, i.e. if you want to change your anvil binary.
// Usage:
//
// From inside an Anvil instance, run 'asm'. It will create a dumpfile of the Anvil state in a special directory. The dumpfile
// filename has the form: <directory>-<title>, where directory is the directory where Anvil was running (with special characters
// like / replaced with -) and title is the window title. This means if you have two Anvil instances running in the same directory
// and they don't have a Title set by the Title command (or have the same Title), then one dump will overwrite the other.
//
// Then close the Anvil instance. Later, from the terminal , run 'ada load' to restart all the saved Anvil sessions.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/jeffwilliams/anvil/api/go/anvil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ogier/pflag"
)

var (
	anvilHttpApi anvil.Anvil
	dumpDir      string
	anvilCwd     string
	anvilTitle   string
	dumpfileName string
)

var (
	optDir = pflag.StringP("dir", "d", "", fmt.Sprintf("Set the directory where dumps are saved to D. By default it is the directory '%s'", calcDefaultDumpDir()))
)

func main() {
	pflag.Usage = usage
	pflag.Parse()

	action, err := action()
	dieIfError(err, fmt.Sprintf("asm: %s", err))

	dumpDir = calcDumpDir()

	switch action {
	case ActionDump:
		dump()
	case ActionLoad:
		load()
	case ActionClear:
		clear()
	case ActionList:
		list()
	}

}

type actionType int

const (
	ActionDump = iota
	ActionLoad
	ActionClear
	ActionList
)

func action() (actionType, error) {
	if pflag.NArg() == 0 {
		return ActionDump, nil
	}

	switch pflag.Arg(0) {
	case "clear":
		return ActionClear, nil
	case "dump":
		return ActionDump, nil
	case "load":
		return ActionLoad, nil
	case "list":
		return ActionList, nil
	default:
		return 0, fmt.Errorf("unknown action '%s'", pflag.Arg(0))
	}
}

func connectToAnvil() {
	var err error
	anvilHttpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")
}

func calcDumpDir() string {
	if *optDir != "" {
		return *optDir
	}

	return calcDefaultDumpDir()
}

func calcDefaultDumpDir() string {
	var d string
	if runtime.GOOS == "windows" {
		d = os.Getenv("USERPROFILE")
	} else {
		d = os.Getenv("HOME")
	}
	d = filepath.Join(d, ".anvil")

	return filepath.Join(d, "asm-dumps")
}

func dump() {

	connectToAnvil()
	info, err := anvilHttpApi.Info()
	dieIfError(err, fmt.Sprintf("asm: getting anvil info failed: %s", err))
	anvilCwd = info.Cwd
	if anvilCwd == "" {
		die("ada: unable to determine Anvil's working directory")
	}
	anvilTitle = info.Title

	os.Mkdir(dumpDir, 0755)

	dumpName, metaName := calcDumpfileAndMetaInfoName()
	dumpPath := filepath.Join(dumpDir, dumpName)
	metaPath := filepath.Join(dumpDir, metaName)

	err = anvilHttpApi.Execute("Dump", []string{dumpPath})
	dieIfError(err, fmt.Sprintf("asm: executing Dump failed: %s", err))

	err = saveMetaInfoFile(metaPath)
	if err != nil {
		os.Remove(dumpPath)
		os.Remove(metaPath)
		dieIfError(err, fmt.Sprintf("asm: saving metainfo failed: %s", err))
	}
}

func saveMetaInfoFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	m := MetaInfo{
		WorkingDir: anvilCwd,
	}

	e := json.NewEncoder(f)
	return e.Encode(m)
}

func load() {
	names, err := dumpfilesInDumpdir()
	dieIfError(err, fmt.Sprintf("asm: listing dumpfiles failed: %s", err))

	for _, name := range names {
		path := filepath.Join(dumpDir, name)

		metaPath := path + ".meta"
		info, err := loadMetaInfoFile(metaPath)
		if err != nil {
			fmt.Printf("asm: error loading metainfo for dumpfile '%s': %v\n", name, err)
		}

		cmd := exec.Command("nohup", "anvil", "-l", path)
		cmd.Dir = info.WorkingDir
		err = cmd.Start()
		if err != nil {
			fmt.Printf("asm: error running Anvil with dumpfile '%s': %v\n", name, err)
		}
	}
}

func list() {
	names, err := dumpfilesInDumpdir()
	dieIfError(err, fmt.Sprintf("asm: listing dumpfiles failed: %s", err))
	for _, name := range names {
		fmt.Printf("%s\n", name)
	}
}

func loadMetaInfoFile(path string) (info MetaInfo, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	e := json.NewDecoder(f)
	err = e.Decode(&info)
	return
}

func dumpfilesInDumpdir() (names []string, err error) {
	ents, err := os.ReadDir(dumpDir)
	if err != nil {
		return nil, err
	}

	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".dump") {
			names = append(names, e.Name())
		}
	}
	return
}

func calcDumpfileAndMetaInfoName() (dumpFile, metaFile string) {

	cleanFilename := func(f string) string {
		f = strings.ReplaceAll(f, "/", "-")
		f = strings.ReplaceAll(f, "\\", "-")
		f = strings.ReplaceAll(f, ":", "-")
		f = strings.ReplaceAll(f, "\t", "-")
		return f
	}

	d := cleanFilename(anvilCwd)

	t := cleanFilename(anvilTitle)

	dumpFile = d + "_" + t + ".dump"
	metaFile = dumpFile + ".meta"

	return
}

func clear() {
	err := os.RemoveAll(dumpDir)
	if err != nil {
		fmt.Printf("asm: removing dump directory '%s' failed: %v\n", dumpDir, err)
	}
}

func usage() {
	fmt.Printf("Usage: %s [options] [action]\n", os.Args[0])
	fmt.Printf("Dump or Load all Anvil sessions. The argument [action] my be set to:\n")
	fmt.Printf("  dump, which saves a Dump of the current Anvil session to a special directory.\n")
	fmt.Printf("  load, which starts Anvil once for each dump file found in the special directory with the\n")
	fmt.Printf("        -l flag set to the dump file, restoring the state to the previous saved value.\n")
	fmt.Printf("  clear, which deletes the dump directory and its contents.\n")
	fmt.Printf("If no action is set, then 'dump' is performed.\n")
	pflag.PrintDefaults()
}

func dieIfError(err error, msg string) {
	if err != nil {
		msg := fmt.Sprintf("%s: %s", msg, err)
		die(msg)
	}
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	os.Exit(1)
}

type MetaInfo struct {
	WorkingDir string
}
