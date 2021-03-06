package util

// TODO(waigani) move util to sdk

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/codegangsta/cli"
	serviceConfig "github.com/codelingo/lingo/service/config"
	goDocker "github.com/fsouza/go-dockerclient"
	"github.com/juju/errors"
	"github.com/mitchellh/go-homedir"
	"go.uber.org/zap"
)

func init() {
	// TODO(waigani) use config/flags to set level etc

	zlog, err := zap.NewProduction()
	if err != nil {
		// yes panic, this is a developer error.
		panic(errors.ErrorStack(err))
	}

	Logger = zlog.Sugar()
}

// TODO(waigani) this is also in common/config can't import due to cycle.
// Refactor util - move into flow cli sdk.
const EnvCfgFile = "lingo-current-env"

func GetEnv() (string, error) {

	configsHome, err := ConfigHome()
	if err != nil {
		return "", errors.Trace(err)
	}
	envFilepath := filepath.Join(configsHome, EnvCfgFile)
	cfg := serviceConfig.New(envFilepath)
	env, err := cfg.GetEnv()
	if err != nil {
		return "", errors.Trace(err)
	}

	return env, nil

}

// Use dev logger rather than prod
func SetDebugLogger() error {
	zlog, err := zap.NewDevelopment()
	if err != nil {
		return errors.Trace(err)
	}
	Logger = zlog.Sugar()
	return nil
}

var Logger *zap.SugaredLogger

// TODO(anyone): Change this back to 'codelingo.yaml' after making config loader check if
//               codelingo.yaml is file (not dir) before reading.
const (
	defaultHome         = ".codelingo"
	DefaultTenetCfgPath = "codelingo.yaml"
)

// OpenFileCmd launches the specified editor at the given filename and line
// number.
func OpenFileCmd(editor, filename string, line int64) (*exec.Cmd, error) {
	app, err := exec.LookPath(editor)
	if err != nil {
		return nil, err
	}

	switch editor {
	case "atom", "subl", "sublime":
		return exec.Command(app, fmt.Sprintf("%s:%d", filename, line)), nil
		// TODO(waigani) other editors?
		// TODO(waigani) make the format a config var
	}

	// Making this default as vi, vim, nano, emacs all do it this way. These
	// are all terminal apps, so take over stdout etc.
	cmd := exec.Command(app, fmt.Sprintf("+%d", line), filename)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd, nil
}

// MustLingoHome returns the path to the user's lingo config directory or
// panics on failure.
func MustLingoHome() string {
	lHome, err := LingoHome()
	if err != nil {
		panic(err)
	}
	return lHome
}

// func LingoConfig() (*config.Config, error) {
// 	cfgPath, err := ConfigHome()
// 	if err != nil {
// 		return errors.Trace(err)
// 	}

// 	return config.New(cfgPath)
// }

// LingoHome returns the path to the user's lingo home directory.
func LingoHome() (string, error) {
	if lHome := os.Getenv("LINGO_HOME"); lHome != "" {
		return lHome, nil
	}
	home, err := UserHome()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, defaultHome), nil
}

// ConfigHome returns the path to the user's lingo config directory.
func ConfigHome() (string, error) {
	if lHome := os.Getenv("LINGO_HOME"); lHome != "" {
		return lHome, nil
	}
	home, err := UserHome()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, defaultHome, "configs"), nil
}

func ConfigDefaults() (string, error) {
	configHome, err := ConfigHome()
	if err != nil {
		return "", errors.Trace(err)
	}
	return filepath.Join(configHome, "defaults"), nil
}

func ConfigUpdates() (string, error) {
	configHome, err := ConfigHome()
	if err != nil {
		return "", errors.Trace(err)
	}
	return filepath.Join(configHome, "updates"), nil
}

// UserHome returns the user's OS home directory.
func UserHome() (string, error) {
	return homedir.Dir()
}

// LingoBin returns the path to where binary tenets are stored.
func LingoBin() (string, error) {
	if bin := os.Getenv("LINGO_HOME"); bin != "" {
		return bin, nil
	}

	lHome, err := LingoHome()
	if err != nil {
		return "", errors.Trace(err)
	}

	return filepath.Join(lHome, "tenets"), nil

}

// BinTenets returns a list of all installed binary tenets as pathnames.
func BinTenets() ([]string, error) {
	binDir, err := LingoBin()
	if err != nil {
		return nil, err
	}

	files, err := filepath.Glob(binDir + "/*/*")
	if err != nil {
		return nil, err
	}

	tenets := make([]string, len(files))
	for i, f := range files {
		f = strings.TrimPrefix(f, binDir+"/")
		tenets[i] = f
	}
	return tenets, nil
}

// TODO(waigani) this is duping the logger in dev. Sort out one solution to
// logging and printing messages and errors.

// Printf provides indirection around the standard fmt.Printf function so that
// the output stream can be globally configured. WARNING: util.Printf is
// deprecated. Prefer tenets/go/dev/tenet/log.Printf.
func Printf(format string, args ...interface{}) (int, error) {
	return Printer.Printf(format, args)
}

// Println provides indirection around the standard fmt.Println function so
// that the output stream can be globally configured. WARNING: util.Println is
// deprecated. Prefer tenets/go/dev/tenet/log.Println.
func Println(line string) {
	Printer.Println(line)
}

func init() {
	Printer = &fmtPrinter{}
}

// Printer is deprecated. Prefer tenets/go/dev/tenet/log.Logger.
var Printer printer

type printer interface {
	Printf(string, ...interface{}) (int, error)
	Println(...interface{}) (int, error)
}

type fmtPrinter struct{}

func (*fmtPrinter) Printf(format string, args ...interface{}) (int, error) {
	return fmt.Printf(format, args...)
}

func (*fmtPrinter) Println(args ...interface{}) (int, error) {
	return fmt.Println(args...)
}

// DockerClient returns a new goDocker client initialised with an endpoint
// specified by the current config.
func DockerClient() (*goDocker.Client, error) {
	// TODO(waigani) get endpoint from ~/.lingo/config.toml
	endpoint := "unix:///var/run/docker.sock"
	return goDocker.NewClient(endpoint)
}

// FormatOutput converts arbitrary data into a string using Go's standard
// template format.
func FormatOutput(in interface{}, tmplt string) (string, error) {
	out := new(bytes.Buffer)
	funcMap := template.FuncMap{
		"join": strings.Join,
	}

	w := tabwriter.NewWriter(out, 0, 8, 1, '\t', 0)
	t := template.Must(template.New("tmpl").Funcs(funcMap).Parse(tmplt))
	err := t.Execute(w, in)
	if err != nil {
		return "", err
	}
	err = w.Flush()
	if err != nil {
		return "", err
	}

	return out.String(), nil
}

var coprcmd = `
#!/bin/bash

# copr: Checkout Pull Request
#
# cd into the repository the pull request targets. The run:
# $ copr <user>/<repo> <branch> [base]
# 
#
# This will: create a new branch; checkout the pull request; and reset any
# commits back to the point the branch forked from base. If base is not set,
# it defaults to master.

set -e

status=` + "`git status -s`" + `
echo $status
if [ -n "$status" ]; then
	echo "aborting: working directory not clean"
	exit
fi

userAndRepo=$1
branch=$2
base="master"

if [ -n "$3" ]; then
	echo "setting base"
	base=$3
fi

name="${userAndRepo/\//-}"

set -x
git co -b $name-$branch master
git pull https://github.com/$userAndRepo.git $branch

sha=` + "`git merge-base --fork-point HEAD $base`" + `

git reset $sha
`[1:]

func MaxArgs(c *cli.Context, max int) error {
	if l := len(c.Args()); l > max {
		return errors.Errorf("expected up to %d argument(s), got %d", max, l)
	}
	return nil
}

// // stderr is a var for mocking in tests
var Stderr io.Writer = os.Stderr

// exiter is a var for mocking in tests
var Exiter = func(code int) {
	os.Exit(code)
}

// DesiredTenetCfgPath returns the tenet config path found in 1. local flag
// or 2. global flag. It falls back to returning DefaultTenetCfgPath
func DesiredTenetCfgPath(c *cli.Context) string {
	flgName := TenetCfgFlg.Long
	var cfgName string
	// 1. grab the config name from local flag
	if cfgName = c.String(flgName); cfgName != "" {
		return cfgName
	}
	if cfgName = c.GlobalString(flgName); cfgName != "" {
		return cfgName
	}
	// TODO(waigani) shouldn't need this - should fallback to default in flags.
	return DefaultTenetCfgPath
}
