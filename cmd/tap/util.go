package main

import (
	"errors"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

func backoff(retries int, max int) time.Duration {
	dur := 1 << retries
	if dur > max {
		dur = max
	}

	jitter := time.Millisecond * time.Duration(rand.Intn(1000))
	return time.Second*time.Duration(dur) + jitter
}

// matchesCollection checks if a collection matches any of the provided filters.
// Filters support wildcards at the end (e.g., "app.bsky.*" & "app.bsky.feed.*" both match "app.bsky.feed.post").
// If no filters are provided, all collections match.
func matchesCollection(collection string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}

	for _, filter := range filters {
		if strings.HasSuffix(filter, "*") {
			prefix := strings.TrimSuffix(filter, "*")
			if strings.HasPrefix(collection, prefix) {
				return true
			}
		} else {
			if collection == filter {
				print("MATCH!! ", filter, " against collection: ", collection, "\n")
				return true
			}
		}
	}

	return false
}

type RecordCollectionFilter struct {
	Collection string
	JSONPath   string
	Pattern    *regexp.Regexp
}

func buildRecordCollectionFilter(filter string) (*RecordCollectionFilter, bool) {
	re := regexp.MustCompile(`^([^\[]+)\[([^\]]+)\]:(.+)$`)
	matches := re.FindStringSubmatch(filter)
	if len(matches) != 4 {
		return nil, false
	}
	collection := matches[1]
	jsonPath := matches[2]
	filterValue := matches[3]
	filterPattern := strings.ReplaceAll(filterValue, "*", ".*")
	patternRe := regexp.MustCompile("^" + filterPattern + "$")
	if patternRe == nil {
		return nil, false
	}

	return &RecordCollectionFilter{
		Collection: collection,
		JSONPath:   jsonPath,
		Pattern:    patternRe,
	}, true
}

func matchesRecordContent(r *Resyncer, collStr string, rec map[string]any, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	collFilterCount := 0
	for _, filter := range filters {
		rcf, ok := buildRecordCollectionFilter(filter)
		if !ok {
			r.logger.Warn("invalid record content filter", "filter", filter)
			continue
		}
		if rcf.Collection == collStr {
			collFilterCount++
		}
		testValue, ok := getByJSONPath(rec, rcf.JSONPath)
		if !ok {
			r.logger.Warn("json path not found in record", "jsonPath", rcf.JSONPath, "collection", collStr)
			continue
		}
		testValueStr, ok := testValue.(string)
		if !ok {
			r.logger.Warn("json path value is not a string", "jsonPath", rcf.JSONPath, "collection", collStr)
			continue
		}
		if rcf.Pattern.MatchString(testValueStr) {
			r.logger.Info(":) MATCHED")
			return true
		}
	}
	if collFilterCount > 0 {
		r.logger.Info(":( NOT MATCHED")
		return false
	} else {
		r.logger.Info(":) MATCHED")
		return true
	}
}

func getByJSONPath(rec map[string]any, jsonPath string) (any, bool) {
	parts := strings.Split(jsonPath, ".")

	var cur any = rec
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func evtHasSignalCollection(evt *comatproto.SyncSubscribeRepos_Commit, signalColl string) bool {
	for _, op := range evt.Ops {
		collection, _, err := syntax.ParseRepoPath(op.Path)
		if err != nil {
			continue
		}
		if collection.String() == signalColl {
			return true
		}
	}
	return false
}

func isRateLimitError(err error) bool {
	var xrpcErr *atclient.APIError
	if errors.As(err, &xrpcErr) {
		return xrpcErr.StatusCode == http.StatusTooManyRequests
	}
	return false
}

func parseOutboxMode(webhookURL string, disableAcks bool) OutboxMode {
	if webhookURL != "" {
		return OutboxModeWebhook
	} else if disableAcks {
		return OutboxModeFireAndForget
	} else {
		return OutboxModeWebsocketAck
	}
}
