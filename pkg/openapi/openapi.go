/*
Copyright (C) 2022-2023 Traefik Labs

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

package openapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/hashicorp/go-version"
	"gopkg.in/yaml.v3"
)

// Loader loads OpenAPI Specifications.
type Loader struct {
	client *http.Client
}

// NewLoader creates a new Loader.
func NewLoader() *Loader {
	return &Loader{
		client: &http.Client{
			Timeout: time.Second * 5,
		},
	}
}

// Load loads the OpenAPI Specification located at the given URL.
func (l *Loader) Load(ctx context.Context, uri *url.URL) (*Spec, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Accept", "application/yaml")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("failed with code %d", resp.StatusCode)
	}

	// Use yaml package to unmarshal both YAML and JSON specification files.
	var spec Spec
	if err = yaml.NewDecoder(resp.Body).Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return &spec, nil
}

// Spec is an OpenAPI Specification.
type Spec struct {
	Swagger string `json:"swagger" yaml:"swagger"`
	OpenAPI string `json:"openapi" yaml:"openapi"`
}

// Validate validates the Specification.
func (s *Spec) Validate() error {
	if s.Swagger != "" {
		return fmt.Errorf("unsupported version %q", s.Swagger)
	}

	v, err := version.NewVersion(s.OpenAPI)
	if err != nil {
		return fmt.Errorf("unsupported version: %q", s.OpenAPI)
	}

	major := v.Segments()[0]
	if major != 3 {
		return fmt.Errorf("unsupported version %q", s.OpenAPI)
	}

	return nil
}
