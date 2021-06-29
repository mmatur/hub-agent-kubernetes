package store

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ldez/go-git-cmd-wrapper/v2/clone"
	"github.com/ldez/go-git-cmd-wrapper/v2/config"
	"github.com/ldez/go-git-cmd-wrapper/v2/git"
	"github.com/ldez/go-git-cmd-wrapper/v2/types"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/platform"
)

// Config represents the topology store config.
type Config struct {
	platform.TopologyConfig

	Token string
}

// Store stores a state in a Git repository.
type Store struct {
	gitRepo     string
	gitExecutor types.Executor
	workingDir  string
}

// New instantiates a new Store.
func New(ctx context.Context, cfg Config) (*Store, error) {
	repoURL := fmt.Sprintf("https://%s:@%s/%s/%s.git", cfg.Token, cfg.GitProxyHost, cfg.GitOrgName, cfg.GitRepoName)

	s := &Store{
		gitRepo:    repoURL,
		workingDir: cfg.GitRepoName,
		gitExecutor: func(ctx context.Context, name string, debug bool, args ...string) (string, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Dir = cfg.GitRepoName

			out, err := cmd.CombinedOutput()
			output := string(out)

			log.Trace().Str("cmd", name).Strs("args", args).Str("output", output).Send()

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
		output, err := git.Config(config.Global, config.Add("http.sslVerify", "false"))
		if err != nil {
			return fmt.Errorf("%w: %s", err, output)
		}
	}

	// Setup local repo for topology files, by cloning hub distant repository
	output, err := git.CloneWithContext(ctx, clone.Repository(s.gitRepo))
	if err != nil {
		if !strings.Contains(output, "already exists and is not an empty directory") {
			return fmt.Errorf("create local repository: %w %s", err, output)
		}
	}

	output, err = git.Config(config.Local, config.Add("user.email", "hubagent@traefik.io"), git.CmdExecutor(s.gitExecutor))
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}

	output, err = git.Config(config.Local, config.Add("user.name", "Hub Agent"), git.CmdExecutor(s.gitExecutor))
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}

	return nil
}

func disableGitSSLVerify() bool {
	_, exists := os.LookupEnv("DISABLE_GIT_SSL_VERIFY")
	return exists
}
