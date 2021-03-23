package reviewer

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/traefik/neo-agent/pkg/acp"
)

const (
	neoSnippetTokenStart = "##neo-snippet-start"
	neoSnippetTokenEnd   = "##neo-snippet-end"
)

type nginxSnippets struct {
	// Community snippets:
	AuthURL              string
	ConfigurationSnippet string
	// Official snippets:
	LocationSnippets string
	ServerSnippets   string
}

func genSnippets(polName string, polCfg *acp.Config, agentAddr string) (nginxSnippets, error) {
	headerToFwd, err := headerToForward(polCfg)
	if err != nil {
		return nginxSnippets{}, fmt.Errorf("get header to forward: %w", err)
	}

	locSnip := generateLocationSnippet(headerToFwd)

	return nginxSnippets{
		AuthURL:              fmt.Sprintf("%s/%s", agentAddr, polName),
		ConfigurationSnippet: wrapNeoSnippet(locSnip),
		LocationSnippets:     wrapNeoSnippet(fmt.Sprintf("auth_request /auth;\n%s", locSnip)),
		ServerSnippets:       wrapNeoSnippet(fmt.Sprintf("location /auth {proxy_pass %s/%s;}", agentAddr, polName)),
	}, nil
}

func headerToForward(cfg *acp.Config) ([]string, error) {
	var headerToFwd []string
	switch {
	case cfg.JWT != nil:
		for headerName := range cfg.JWT.ForwardHeaders {
			headerToFwd = append(headerToFwd, headerName)
		}

		if cfg.JWT.StripAuthorizationHeader {
			headerToFwd = append(headerToFwd, "Authorization")
		}

		return headerToFwd, nil
	default:
		return nil, errors.New("unsupported ACP type")
	}
}

func generateLocationSnippet(headerToForward []string) string {
	var location string
	for i, header := range headerToForward {
		location += fmt.Sprintf("auth_request_set $value_%d $upstream_http_%s; ", i, strings.ReplaceAll(header, "-", "_"))
		location += fmt.Sprintf("proxy_set_header %s $value_%d;\n", header, i)
	}
	return location
}

func wrapNeoSnippet(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("%s\n%s\n%s", neoSnippetTokenStart, strings.TrimSpace(s), neoSnippetTokenEnd)
}

func mergeSnippets(snippets nginxSnippets, anno map[string]string) nginxSnippets {
	return nginxSnippets{
		AuthURL:              snippets.AuthURL,
		ConfigurationSnippet: mergeSnippet(anno["nginx.ingress.kubernetes.io/configuration-snippet"], snippets.ConfigurationSnippet),
		LocationSnippets:     mergeSnippet(anno["nginx.org/location-snippets"], snippets.LocationSnippets),
		ServerSnippets:       mergeSnippet(anno["nginx.org/server-snippets"], snippets.ServerSnippets),
	}
}

var re = regexp.MustCompile(fmt.Sprintf(`(?ms)^(.*)(%s.*%s)(.*)$`, neoSnippetTokenStart, neoSnippetTokenEnd))

func mergeSnippet(oldSnippet, neoSnippet string) string {
	matches := re.FindStringSubmatch(oldSnippet)
	if len(matches) == 4 {
		return matches[1] + neoSnippet + matches[3]
	}

	if oldSnippet != "" && neoSnippet != "" {
		return oldSnippet + "\n" + neoSnippet
	}
	return oldSnippet + neoSnippet
}
