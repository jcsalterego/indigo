package search

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	api "github.com/bluesky-social/indigo/api"
	bsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/labstack/echo/v4"
	otel "go.opentelemetry.io/otel"
)

var /* const */ FromOperatorRegexp = regexp.MustCompile(`\bfrom:([\w\.]+)`)

type ActorSearchResp struct {
	bsky.ActorProfile
	DID string `json:"did"`
}

func (s *Server) handleFromDid(ctx context.Context, did string) (string, error) {
	phr := &api.ProdHandleResolver{}
	handle, _, err := api.ResolveDidToHandle(ctx, s.xrpcc, s.plc, phr, did)
	if err != nil {
		return "", err
	}

	return handle, nil
}

func (s *Server) handleSearchRequestPosts(e echo.Context) error {
	ctx, span := otel.Tracer("search").Start(e.Request().Context(), "handleSearchRequestPosts")
	defer span.End()

	queryParam := strings.TrimSpace(e.QueryParam("q"))
	if queryParam == "" {
		return e.JSON(400, map[string]any{
			"error": "must pass non-empty search query",
		})
	}

	// if the query contains 'from:username.foo.tld',
	// extract username.foo.tld, and remove the entire operator token
	fromHandle := ""
	matches := FromOperatorRegexp.FindStringSubmatch(queryParam)
	if len(matches) == 2 {
		fromHandle = matches[1]
		queryParam = strings.TrimSpace(
			FromOperatorRegexp.ReplaceAllString(queryParam, ""),
		)
	}

	offset := 0
	if offsetParam := strings.TrimSpace(e.QueryParam("offset")); offsetParam != "" {
		v, err := strconv.Atoi(offsetParam)
		if err != nil {
			return &echo.HTTPError{
				Code:    400,
				Message: fmt.Sprintf("invalid value for 'offset': %s", err),
			}
		}

		offset = v
	}

	count := 30
	if countParam := strings.TrimSpace(e.QueryParam("count")); countParam != "" {
		v, err := strconv.Atoi(countParam)
		if err != nil {
			return &echo.HTTPError{
				Code:    400,
				Message: fmt.Sprintf("invalid value for 'count': %s", err),
			}
		}

		count = v
	}

	out, err := s.SearchPosts(ctx, queryParam, fromHandle, offset, count)
	if err != nil {
		return err
	}

	return e.JSON(200, out)
}

func (s *Server) handleSearchRequestProfiles(e echo.Context) error {
	ctx, span := otel.Tracer("search").Start(e.Request().Context(), "handleSearchRequestProfiles")
	defer span.End()

	q := strings.TrimSpace(e.QueryParam("q"))
	if q == "" {
		return e.JSON(400, map[string]any{
			"error": "must pass non-empty search query",
		})
	}

	out, err := s.SearchProfiles(ctx, q)
	if err != nil {
		return err
	}

	return e.JSON(200, out)
}
