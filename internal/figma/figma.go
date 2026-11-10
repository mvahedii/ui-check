package figma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Link struct {
	FileKey string
	NodeID  string
}

var (
	fileKeyPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	nodeIDPattern  = regexp.MustCompile(`^[0-9]+[:-][0-9]+$`)
)

func ParseLink(raw string) (Link, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || (u.Host != "www.figma.com" && u.Host != "figma.com") || u.User != nil {
		return Link{}, errors.New("expected an HTTPS figma.com design link")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || (parts[0] != "design" && parts[0] != "file") || !fileKeyPattern.MatchString(parts[1]) {
		return Link{}, errors.New("invalid Figma file key or path")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Link{}, errors.New("invalid Figma link query")
	}
	link := Link{FileKey: parts[1]}
	if ids, ok := query["node-id"]; ok {
		if len(ids) != 1 || !nodeIDPattern.MatchString(ids[0]) {
			return Link{}, errors.New("invalid Figma node-id")
		}
		link.NodeID = strings.ReplaceAll(ids[0], "-", ":")
	}
	return link, nil
}

type Node struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Box  *struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"absoluteBoundingBox"`
}

type APIError struct {
	Status     int
	RetryAfter string
}

func (e *APIError) Error() string {
	switch e.Status {
	case 401:
		return "Figma rejected authentication; check FIGMA_ACCESS_TOKEN"
	case 403:
		return "Figma denied access; check file permissions and file_content:read"
	case 404:
		return "Figma file or node not found"
	case 429:
		return "Figma rate limit reached; wait before retrying or use a saved snapshot"
	default:
		return fmt.Sprintf("Figma API returned HTTP %d", e.Status)
	}
}

type Client struct {
	base string
	http *http.Client
}

func NewClient() Client {
	return Client{
		base: "https://api.figma.com",
		http: &http.Client{
			Timeout:       15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c Client) LoadNode(ctx context.Context, link Link, token string) (Node, error) {
	if token == "" {
		return Node{}, errors.New("set FIGMA_ACCESS_TOKEN")
	}
	if !fileKeyPattern.MatchString(link.FileKey) || !nodeIDPattern.MatchString(link.NodeID) {
		return Node{}, errors.New("a valid Figma file key and node-id are required")
	}
	query := url.Values{"ids": {link.NodeID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/files/"+link.FileKey+"/nodes?"+query.Encode(), nil)
	if err != nil {
		return Node{}, errors.New("could not prepare Figma request")
	}
	req.Header.Set("X-Figma-Token", token)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Node{}, ctx.Err()
		}
		return Node{}, errors.New("Figma request failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Node{}, &APIError{Status: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	const maxBody = 8 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return Node{}, errors.New("could not read Figma response within size limit")
	}
	var decoded struct {
		Nodes map[string]struct {
			Document Node `json:"document"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Node{}, errors.New("invalid Figma node response")
	}
	node := decoded.Nodes[link.NodeID].Document
	if node.ID != link.NodeID || node.Box == nil {
		return Node{}, errors.New("Figma response did not include the requested node geometry")
	}
	return node, nil
}
