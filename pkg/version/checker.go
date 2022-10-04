/*
Copyright (C) 2022 Traefik Labs

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package version

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/go-github/v47/github"
	goversion "github.com/hashicorp/go-version"
	"github.com/rs/zerolog/log"
)

type clusterService interface {
	SetVersionStatus(ctx context.Context, state Status) error
}

// Status holds agent version data.
type Status struct {
	UpToDate       bool   `json:"upToDate,omitempty"`
	CurrentVersion string `json:"currentVersion,omitempty"`
	LatestVersion  string `json:"latestVersion,omitempty"`
}

// addHeaderTransport allows to add header to http request.
type addHeaderTransport struct {
	http.RoundTripper
}

// RoundTrip add headers to http request.
func (adt *addHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Traefik-Hub-Agent-Version", version)
	req.Header.Add("Traefik-Hub-Agent-Platform", "kubernetes")

	return adt.RoundTripper.RoundTrip(req)
}

func newGitHubClient(baseURL *url.URL) *github.Client {
	client := github.NewClient(&http.Client{Transport: &addHeaderTransport{RoundTripper: http.DefaultTransport}})
	client.UserAgent = userAgent()
	client.BaseURL = baseURL

	return client
}

// Checker is able to check the agent version.
type Checker struct {
	cluster clusterService
	github  *github.Client
	version string
}

// NewChecker returns a new Checker.
func NewChecker(cluster clusterService) *Checker {
	baseURL, _ := url.Parse("https://update.traefik.io/")

	return &Checker{
		cluster: cluster,
		github:  newGitHubClient(baseURL),
		version: version,
	}
}

// Start starts the check of the agent version.
func (c Checker) Start(ctx context.Context) error {
	tick := time.NewTicker(24 * time.Hour)
	defer tick.Stop()

	time.Sleep(10 * time.Minute)

	if err := c.check(ctx); err != nil {
		log.Warn().Err(err).Msg("check new version")
	}

	for {
		select {
		case <-tick.C:
			if err := c.check(ctx); err != nil {
				log.Warn().Err(err).Msg("check new version")
			}

		case <-ctx.Done():
			return nil
		}
	}
}

// check Checks if a new version is available.
func (c Checker) check(ctx context.Context) error {
	if c.version == defaultVersion {
		return nil
	}

	status, err := c.getStatus(ctx)
	if err != nil {
		return fmt.Errorf("get version status: %w", err)
	}

	err = c.cluster.SetVersionStatus(ctx, status)
	if err != nil {
		return fmt.Errorf("set version status: %w", err)
	}

	if !status.UpToDate {
		return fmt.Errorf("you are using %s version of the agent, please consider upgrading to %s", status.CurrentVersion, status.LatestVersion)
	}

	return nil
}

func (c Checker) getStatus(ctx context.Context) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tags, resp, err := c.github.Repositories.ListTags(ctx, "traefik", "hub-agent-kubernetes", nil)
	if err != nil {
		return Status{}, fmt.Errorf("list tags: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		all, _ := io.ReadAll(resp.Body)

		return Status{}, fmt.Errorf("list tags: %s", string(all))
	}

	latestVersion, err := goversion.NewSemver(tags[0].GetName())
	if err != nil {
		return Status{}, fmt.Errorf("parse version: %w", err)
	}

	currentVersion, err := goversion.NewSemver(c.version)
	// not a valid tag.
	if err != nil {
		return Status{
			CurrentVersion: c.version,
			LatestVersion:  latestVersion.Original(),
		}, nil
	}

	// outdated version.
	if latestVersion.GreaterThan(currentVersion) {
		return Status{
			CurrentVersion: c.version,
			LatestVersion:  latestVersion.Original(),
		}, nil
	}

	return Status{
		UpToDate:       true,
		CurrentVersion: c.version,
		LatestVersion:  c.version,
	}, nil
}
