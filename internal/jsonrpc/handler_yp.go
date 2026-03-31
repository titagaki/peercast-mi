package jsonrpc

import "strings"

type ypEntry struct {
	YellowPageID int    `json:"yellowPageId"`
	Name         string `json:"name"`
	URI          string `json:"uri"`
	AnnounceURI  string `json:"announceUri"`
}

func (s *Server) getYellowPages() (interface{}, *rpcError) {
	result := make([]ypEntry, len(s.cfg.YPs))
	for i, entry := range s.cfg.YPs {
		uri := entry.Addr
		if !strings.Contains(uri, "://") {
			uri = "pcp://" + uri + "/"
		}
		result[i] = ypEntry{
			YellowPageID: i,
			Name:         entry.Name,
			URI:          uri,
			AnnounceURI:  uri,
		}
	}
	return result, nil
}
