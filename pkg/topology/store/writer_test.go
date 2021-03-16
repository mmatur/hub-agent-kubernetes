package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/neo-agent/pkg/topology/state"
)

const (
	commitCommand = "commit"
	pushCommand   = "push"
)

func TestWrite_GitNoChanges(t *testing.T) {
	tmpDir := t.TempDir()

	var pushCalled bool
	s := &Store{
		workingDir: tmpDir,
		gitExecutor: func(_ context.Context, _ string, _ bool, args ...string) (string, error) {
			switch args[0] {
			case commitCommand:
				return "nothing to commit", errors.New("fake error")
			case pushCommand:
				pushCalled = true
			}

			return "", nil
		},
	}

	err := s.Write(context.Background(), &state.Cluster{ID: "myclusterID"})
	require.NoError(t, err)

	assert.False(t, pushCalled)
}

func TestWrite_GitChanges(t *testing.T) {
	tmpDir := t.TempDir()

	var pushCalled bool
	s := &Store{
		workingDir: tmpDir,
		gitExecutor: func(_ context.Context, _ string, _ bool, args ...string) (string, error) {
			if args[0] == pushCommand {
				pushCalled = true
			}
			return "", nil
		},
	}

	err := s.Write(context.Background(), &state.Cluster{ID: "myclusterID"})
	require.NoError(t, err)

	assert.True(t, pushCalled)
}

func TestWrite_Apps(t *testing.T) {
	tmpDir := t.TempDir()

	app := &state.App{Name: "mysvc"}

	var pushCalled bool
	s := &Store{
		workingDir: tmpDir,
		gitExecutor: func(_ context.Context, _ string, _ bool, args ...string) (string, error) {
			if args[0] == pushCommand {
				pushCalled = true
			}
			return "", nil
		},
	}

	err := s.Write(context.Background(), &state.Cluster{
		ID: "myclusterID",
		Apps: map[string]*state.App{
			"daemonSet/mysvc@myns": app,
		},
	})
	require.NoError(t, err)

	assert.True(t, pushCalled)

	got := readTopology(t, tmpDir)

	var gotApp state.App
	err = json.Unmarshal(got["/Apps/daemonSet/mysvc@myns.json"], &gotApp)
	require.NoError(t, err)

	assert.Equal(t, app, &gotApp)
}

func TestWrite_Namespaces(t *testing.T) {
	tmpDir := t.TempDir()

	var pushCalled bool
	s := &Store{
		workingDir: tmpDir,
		gitExecutor: func(_ context.Context, _ string, _ bool, args ...string) (string, error) {
			if args[0] == pushCommand {
				pushCalled = true
			}
			return "", nil
		},
	}

	err := s.Write(context.Background(), &state.Cluster{
		ID:         "myclusterID",
		Namespaces: []string{"titi", "toto"},
	})
	require.NoError(t, err)

	assert.True(t, pushCalled)

	got := readTopology(t, tmpDir)

	assert.Contains(t, got, "/Namespaces/titi")
	assert.Contains(t, got, "/Namespaces/toto")
	assert.Len(t, got, 3)
}

func TestWrite_IngressControllers(t *testing.T) {
	tmpDir := t.TempDir()

	testController := &state.IngressController{Name: "myctrl"}

	var pushCalled bool
	s := &Store{
		workingDir: tmpDir,
		gitExecutor: func(_ context.Context, _ string, _ bool, args ...string) (string, error) {
			if args[0] == pushCommand {
				pushCalled = true
			}
			return "", nil
		},
	}

	err := s.Write(context.Background(), &state.Cluster{
		ID: "myclusterID",
		IngressControllers: map[string]*state.IngressController{
			"myctrl@myns": testController,
		},
	})
	require.NoError(t, err)

	assert.True(t, pushCalled)

	got := readTopology(t, tmpDir)

	var gotCtrl state.IngressController
	err = json.Unmarshal(got["/IngressControllers/myctrl@myns.json"], &gotCtrl)
	require.NoError(t, err)

	assert.Equal(t, testController, &gotCtrl)
}

func readTopology(t *testing.T, dir string) map[string][]byte {
	t.Helper()

	result := make(map[string][]byte)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			if path == "./" {
				return nil
			}

			data, err := os.ReadFile(path)
			require.NoError(t, err)

			result[strings.TrimPrefix(path, dir)] = data
		}
		return nil
	})
	require.NoError(t, err)

	return result
}
