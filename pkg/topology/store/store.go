package store

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ldez/go-git-cmd-wrapper/v2/clone"
	cfg "github.com/ldez/go-git-cmd-wrapper/v2/config"
	"github.com/ldez/go-git-cmd-wrapper/v2/git"
	"github.com/ldez/go-git-cmd-wrapper/v2/types"
	"github.com/rs/zerolog/log"
)

// Store stores a state in a Git repository.
type Store struct {
	gitRepo     string
	gitExecutor types.Executor
	workingDir  string
}

// New instantiates a new Store.
func New(ctx context.Context, token, topologyServiceURL string) (*Store, error) {
	config, err := fetchConfig(ctx, token, topologyServiceURL)
	if err != nil {
		return nil, err
	}

	path, err := repositoryName(config.GitRepo)
	if err != nil {
		return nil, err
	}

	s := &Store{
		gitRepo:    config.GitRepo,
		workingDir: path,
		gitExecutor: func(ctx context.Context, name string, debug bool, args ...string) (string, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Dir = path

			out, err := cmd.CombinedOutput()
			output := string(out)

			log.Debug().Str("cmd", name).Strs("args", args).Str("output", output).Send()

			return output, err
		},
	}

	if err := s.cloneRepository(ctx); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) cloneRepository(ctx context.Context) error {
	if disableGitSSLVerify() {
		log.Info().Msg("Git SSL verify disabled")
		output, err := git.Config(cfg.Global, cfg.Add("http.sslVerify", "false"))
		if err != nil {
			return fmt.Errorf("%w: %s", err, output)
		}
	}

	// Setup local repo for topology files, by cloning neo distant repository
	output, err := git.CloneWithContext(ctx, clone.Repository(s.gitRepo))
	if err != nil {
		if !strings.Contains(output, "already exists and is not an empty directory") {
			return fmt.Errorf("create local repository: %w %s", err, output)
		}
	}

	output, err = git.Config(cfg.Local, cfg.Add("user.email", "neoagent@traefik.io"), git.CmdExecutor(s.gitExecutor))
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}

	output, err = git.Config(cfg.Local, cfg.Add("user.name", "Neo Agent"), git.CmdExecutor(s.gitExecutor))
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}

	return nil
}

func repositoryName(gitRepo string) (string, error) {
	repo := strings.TrimSuffix(gitRepo, ".git")

	index := strings.LastIndex(repo, "/")
	if index == -1 || index == len(repo)-1 {
		return "", fmt.Errorf("malformed git repo URL: %s", gitRepo)
	}

	return repo[index+1:], nil
}

func disableGitSSLVerify() bool {
	_, exists := os.LookupEnv("DISABLE_GIT_SSL_VERIFY")
	return exists
}
