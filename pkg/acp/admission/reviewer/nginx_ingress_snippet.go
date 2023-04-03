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

package reviewer

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
)

const (
	hubSnippetTokenStart = "##hub-snippet-start" //nolint:gosec // This is not hardcoded credentials.
	hubSnippetTokenEnd   = "##hub-snippet-end"   //nolint:gosec // This is not hardcoded credentials.

	authURL              = "nginx.ingress.kubernetes.io/auth-url"
	authSignin           = "nginx.ingress.kubernetes.io/auth-signin"
	authSnippet          = "nginx.ingress.kubernetes.io/auth-snippet"
	configurationSnippet = "nginx.ingress.kubernetes.io/configuration-snippet"
	serverSnippet        = "nginx.ingress.kubernetes.io/server-snippet"
)

func genNginxAnnotations(polName string, polCfg *acp.Config, agentAddr, groups string) (map[string]string, error) {
	// If there's no policy given, force a 404 response. It allows to untie ACP creation from ACP reference and
	// remove ordering constraints while still not exposing publicly a protected resource.
	if polCfg == nil {
		return map[string]string{
			configurationSnippet: wrapHubSnippet("return 404;"),
		}, nil
	}

	headerToFwd, err := headerToForward(polCfg)
	if err != nil {
		return nil, fmt.Errorf("get header to forward: %w", err)
	}

	locSnip := generateLocationSnippet(headerToFwd)

	if polCfg.OIDC == nil {
		address := fmt.Sprintf("%s/%s", agentAddr, polName)
		if groups != "" {
			address += "?groups=" + url.QueryEscape(groups)
		}

		return map[string]string{
			authURL:              address,
			configurationSnippet: wrapHubSnippet(locSnip),
		}, nil
	}

	redirectPath, err := redirectPath(polCfg)
	if err != nil {
		return nil, err
	}

	headers := `
proxy_set_header From nginx;
proxy_set_header X-Forwarded-Uri $request_uri;
proxy_set_header X-Forwarded-Host $host;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-Method $request_method;`
	authServerURL := fmt.Sprintf("%s/%s", agentAddr, polName)

	return map[string]string{
		authURL:              authServerURL,
		authSignin:           "$url_redirect",
		authSnippet:          wrapHubSnippet(headers),
		configurationSnippet: wrapHubSnippet(locSnip + " auth_request_set $url_redirect $upstream_http_url_redirect;"),
		serverSnippet:        wrapHubSnippet(fmt.Sprintf("location %s { proxy_pass %s; %s}", redirectPath, authServerURL, headers)),
	}, nil
}

func redirectPath(polCfg *acp.Config) (string, error) {
	u, err := url.Parse(polCfg.OIDC.RedirectURL)
	if err != nil {
		return "", fmt.Errorf("parse redirect url: %w", err)
	}

	redirectPath := u.Path
	if redirectPath == "" {
		redirectPath = "/callback"
	}

	if redirectPath[0] != '/' {
		redirectPath = "/" + redirectPath
	}

	return redirectPath, nil
}

func generateLocationSnippet(headerToForward []string) string {
	var location string
	for i, header := range headerToForward {
		location += fmt.Sprintf("auth_request_set $value_%d $upstream_http_%s; ", i, strings.ReplaceAll(header, "-", "_"))
		location += fmt.Sprintf("proxy_set_header %s $value_%d;\n", header, i)
	}

	return location
}

func wrapHubSnippet(s string) string {
	if s == "" {
		return ""
	}

	return fmt.Sprintf("%s\n%s\n%s", hubSnippetTokenStart, strings.TrimSpace(s), hubSnippetTokenEnd)
}

func mergeSnippets(nginxAnno, anno map[string]string) map[string]string {
	nginxAnno[authSnippet] = mergeSnippet(anno[authSnippet], nginxAnno[authSnippet])
	nginxAnno[configurationSnippet] = mergeSnippet(anno[configurationSnippet], nginxAnno[configurationSnippet])
	nginxAnno[serverSnippet] = mergeSnippet(anno[serverSnippet], nginxAnno[serverSnippet])

	return nginxAnno
}

var re = regexp.MustCompile(fmt.Sprintf(`(?ms)^(.*)(%s.*%s)(.*)$`, hubSnippetTokenStart, hubSnippetTokenEnd))

func mergeSnippet(oldSnippet, hubSnippet string) string {
	matches := re.FindStringSubmatch(oldSnippet)
	if len(matches) == 4 {
		return matches[1] + hubSnippet + matches[3]
	}

	if oldSnippet != "" && hubSnippet != "" {
		return oldSnippet + "\n" + hubSnippet
	}

	return oldSnippet + hubSnippet
}
