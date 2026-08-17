package routes

import (
	"omnillm/internal/cif"
	"omnillm/internal/lib/responsecache"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

const responseCacheStateKey = "responsecache_state"

type responseCacheState struct {
	config responsecache.Config
	key    string
}

func sanitizeResponseCacheHit(response *cif.CanonicalResponse, request *cif.CanonicalRequest) *cif.CanonicalResponse {
	response = normalizeCachedToolArguments(response, request)
	if response == nil || response.Usage == nil {
		return response
	}
	clone := *response
	usage := *response.Usage
	usage.UncachedInputTokens = nil
	usage.CacheReadInputTokens = nil
	usage.CacheWriteInputTokens = nil
	usage.CacheWrite5mInputTokens = nil
	usage.CacheWrite1hInputTokens = nil
	clone.Usage = &usage
	return &clone
}

func lookupResponseCache(c *gin.Context, request *cif.CanonicalRequest, requestID string) *cif.CanonicalResponse {
	cfg := responsecache.LoadConfig()
	bypass := responsecache.ParseBypass(c.GetHeader(responsecache.BypassHeader))
	if !cfg.Enabled || bypass == responsecache.BypassAll {
		return nil
	}

	key, err := responsecache.Key(request)
	if err != nil {
		log.Warn().Err(err).Str("request_id", requestID).Msg("Response cache key construction failed; falling through to upstream")
		return nil
	}
	c.Set(responseCacheStateKey, responseCacheState{config: cfg, key: key})
	if bypass == responsecache.BypassRead {
		return nil
	}
	return sanitizeResponseCacheHit(responsecache.GetContext(c.Request.Context(), cfg, request, key), request)
}

func responseCacheMissState(c *gin.Context) (responseCacheState, bool) {
	value, ok := c.Get(responseCacheStateKey)
	if !ok {
		return responseCacheState{}, false
	}
	state, ok := value.(responseCacheState)
	return state, ok && state.key != ""
}

func populateResponseCache(c *gin.Context, request *cif.CanonicalRequest, response *cif.CanonicalResponse) {
	state, ok := responseCacheMissState(c)
	if !ok {
		return
	}
	responsecache.PutContext(c.Request.Context(), state.config, request, state.key, response)
	c.Header(responsecache.BypassHeader, "miss")
}

func newResponseCacheStreamAccumulator(c *gin.Context) (*responsecache.StreamAccumulator, responseCacheState) {
	state, ok := responseCacheMissState(c)
	if !ok {
		return nil, responseCacheState{}
	}
	c.Header(responsecache.BypassHeader, "miss")
	return responsecache.NewStreamAccumulator(), state
}

func populateResponseCacheStream(c *gin.Context, request *cif.CanonicalRequest, accumulator *responsecache.StreamAccumulator, state responseCacheState) {
	if accumulator == nil {
		return
	}
	if assembled := accumulator.Response(); assembled != nil {
		responsecache.PutContext(c.Request.Context(), state.config, request, state.key, assembled)
	}
}
