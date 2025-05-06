package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type DbGenerator struct {
	// workspaceRoot is the root directory where the compile_commands.json file will be created
	workspaceRoot string
	// buildDir is the directory where the compiler commands are actually run
	buildDir string
	// extraArgs are extra arguments to insert into the compilation command between the compiler binary and the filename
	extraArgs string
}

func NewDbGenerator(workspaceRoot, buildDir string) (gen DbGenerator, err error) {
	workspaceRoot, err = filepath.Abs(workspaceRoot)
	if err != nil {
		return
	}
	buildDir, err = filepath.Abs(buildDir)
	if err != nil {
		return
	}

	return DbGenerator{workspaceRoot, buildDir, ""}, err
}

// Add walks all the files and directories under dir and adds them to the compilation db
func (g *DbGenerator) Add(dir string) error {
	db, err := g.loadDb()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	dir, err = filepath.Abs(dir)
	if err != nil {
		return err
	}

	err = g.walkAndAdd(dir, &db)
	if err != nil {
		return err
	}

	return g.saveDb(db)
}

func (g *DbGenerator) loadDb() (db Db, err error) {
	f, err := os.Open(g.dbPath())
	if err != nil {
		return
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	entries := []Entry{}
	err = dec.Decode(&entries)
	if err != nil {
		return
	}
	for _, e := range entries {
		db.Add(e)
	}
	return
}

func (g *DbGenerator) dbPath() string {
	return filepath.Join(g.workspaceRoot, "compile_commands.json")
}

func (g *DbGenerator) walkAndAdd(dir string, db *Db) error {
	fmt.Printf("walking dir %s\n", dir)
	err := fs.WalkDir(os.DirFS(dir), ".", func(path string, d fs.DirEntry, err error) error {
		//if strings.HasSuffix(path, ".c") || strings.HasSuffix(path, ".cpp") || strings.HasSuffix(path, ".h") {
		if strings.HasSuffix(path, ".c") || strings.HasSuffix(path, ".cpp") {
			fmt.Printf("found file %s\n", path)
			//d, f := filepath.Split(path)

			args := ""
			if g.extraArgs != "" {
				args = fmt.Sprintf("%s ", g.extraArgs)
			}

			file := filepath.Join(dir, path)
			buildDir := g.buildDirFor(file)
			relFilePath, err := g.filePathFromBuildDir(buildDir, file)
			if err != nil {
				return nil
			}

			e := Entry{
				Directory: buildDir,
				Command:   fmt.Sprintf("clang %s%s", args, relFilePath),
				File:      relFilePath,
			}

			db.Add(e)
		}
		return nil
	})

	if err != nil {
		return err
	}

	return err
}

func (g *DbGenerator) buildDirFor(file string) string {
	if g.buildDir == "" {
		d, _ := filepath.Split(file)
		return d
	}
	return g.buildDir
}

func (g *DbGenerator) filePathFromBuildDir(buildDir, file string) (string, error) {
	return filepath.Rel(buildDir, file)
}

func (g *DbGenerator) saveDb(db Db) (err error) {
	f, err := os.Create(g.dbPath())
	if err != nil {
		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(db.Slice())
	return
}

type Db struct {
	entries map[DbKey]Entry
}

type DbKey struct {
	Directory, File string
}

func (db *Db) Add(e Entry) {
	if db.entries == nil {
		db.entries = make(map[DbKey]Entry)
	}
	db.entries[DbKey{e.Directory, e.File}] = e
}

func (db Db) Slice() []Entry {
	r := []Entry{}
	for _, v := range db.entries {
		r = append(r, v)
	}
	return r
}

type Entry struct {
	Directory string `json:"directory"`
	Command   string `json:"command"`
	File      string `json:"file"`
}
