package settings

import "github.com/Zazza/imagecaster/internal/serverclient"

const (
	ServerOK            = "ok"
	ServerNotConfigured = "not_configured"
	ServerNotFound      = "not_found"
	ServerOutdated      = "outdated"
)

type ServerCheck struct {
	State   string   `json:"state"`
	URL     string   `json:"url"`
	Version string   `json:"version,omitempty"`
	Missing []string `json:"missing,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func (s *Service) CheckServer() ServerCheck {
	u, _ := s.db.GetSetting("server_url")
	if u == "" {
		return ServerCheck{State: ServerNotConfigured}
	}
	info, err := serverclient.Probe(u)
	if err != nil {
		return ServerCheck{State: ServerNotFound, URL: u, Error: err.Error()}
	}
	if missing := info.MissingFeatures(); len(missing) > 0 {
		return ServerCheck{State: ServerOutdated, URL: u, Version: info.Version, Missing: missing}
	}
	return ServerCheck{State: ServerOK, URL: u, Version: info.Version}
}
