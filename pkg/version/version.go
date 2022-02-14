package version

import (
	"fmt"
	"html/template"
	"io"
	"runtime"
	"runtime/debug"

	"github.com/rs/zerolog/log"
)

// These variables are set when compiling the project.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

var versionTemplate = `Version:      {{.Version}}
Commit:       {{ .Commit }}
Go version:   {{.GoVersion}}
Built:        {{.BuildTime}}
OS/Arch:      {{.Os}}/{{.Arch}}
`

// Print prints the full version information on the given writer.
func Print(w io.Writer) error {
	tmpl, err := template.New("").Parse(versionTemplate)
	if err != nil {
		return err
	}

	v := struct {
		Version   string
		Commit    string
		BuildTime string
		GoVersion string
		Os        string
		Arch      string
	}{
		Version:   version,
		Commit:    commit,
		BuildTime: date,
		GoVersion: runtime.Version(),
		Os:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}

	return tmpl.Execute(w, v)
}

// String returns a quick summary of version information.
func String() string {
	return fmt.Sprintf("%s, build %s on %s", version, commit, date)
}

// Log logs the full version information.
func Log() {
	log.Info().
		Str("version", version).
		Str("module", moduleName()).
		Str("commit", commit).
		Str("built", date).
		Str("go_version", runtime.Version()).
		Str("os", runtime.GOOS).
		Str("arch", runtime.GOARCH).
		Send()
}

func moduleName() string {
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		return buildInfo.Main.Path
	}
	return ""
}
