package batcher

import (
	"hash/fnv"
	"time"

	"distributed-llm-inference-router/replica/cache"
	"distributed-llm-inference-router/replica/model"
)

type Config struct {
	MaxBatch     int
	TokenBudget  int
	TickInterval time.Duration
	PrefixLen    int
}

type Request struct {
	ID           string
	PromptTokens []int32
	MaxNewTokens int32
	ResultCh     chan Result
}

type Result struct {
	OutputTokens  []int32
	CacheHit      bool
	PrefillTokens int64
	Err           error
}

type sequence struct {
	req           *Request
	prefillTicks  int
	decodeTicks   int
	generated     []int32
	cacheHit      bool
	prefillTokens int64
}

type Batcher struct {
	cfg      Config
	model    *model.MockModel
	cache    *cache.KVCache
	incoming chan *Request
	stop     chan struct{}
	done     chan struct{}
}

func New(cfg Config, m *model.MockModel, kv *cache.KVCache) *Batcher {
	return &Batcher{
		cfg:      cfg,
		model:    m,
		cache:    kv,
		incoming: make(chan *Request, 256),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (b *Batcher) Submit(req *Request) { b.incoming <- req }

func (b *Batcher) Start() { go b.loop() }

func (b *Batcher) Stop() {
	close(b.stop)
	<-b.done
}

func hashPrefix(tokens []int32, n int) uint64 {
	if n > len(tokens) {
		n = len(tokens)
	}
	h := fnv.New64a()
	for _, t := range tokens[:n] {
		h.Write([]byte{byte(t), byte(t >> 8), byte(t >> 16), byte(t >> 24)})
	}
	return h.Sum64()
}

func durToTicks(d, tick time.Duration) int {
	if tick <= 0 || d <= 0 {
		return 0
	}
	t := int(d / tick)
	if t < 1 {
		t = 1
	}
	return t
}

func (b *Batcher) loop() {
	defer close(b.done)
	ticker := time.NewTicker(b.cfg.TickInterval)
	defer ticker.Stop()

	var running []*sequence
	tokensInFlight := 0

	for {
		select {
		case <-b.stop:
			return
		case <-ticker.C:
			// Admit waiting requests up to MaxBatch and TokenBudget.
			for len(running) < b.cfg.MaxBatch && tokensInFlight < b.cfg.TokenBudget {
				var req *Request
				select {
				case req = <-b.incoming:
				default:
					goto step
				}
				hash := hashPrefix(req.PromptTokens, b.cfg.PrefixLen)
				prefixLen, hit := b.cache.Lookup(hash)
				uncached := len(req.PromptTokens)
				if hit {
					if uncached > prefixLen {
						uncached = uncached - prefixLen
					} else {
						uncached = 0
					}
				}
				if !hit {
					b.cache.Store(hash, len(req.PromptTokens))
				}
				costs := b.model.Costs(uncached, int(req.MaxNewTokens))
				seq := &sequence{
					req:           req,
					prefillTicks:  durToTicks(costs.Prefill, b.cfg.TickInterval),
					decodeTicks:   durToTicks(costs.Decode, b.cfg.TickInterval),
					cacheHit:      hit,
					prefillTokens: int64(uncached),
				}
				running = append(running, seq)
				tokensInFlight += len(req.PromptTokens) + int(req.MaxNewTokens)
			}
		step:
			// Decrement ticks; evict completed sequences.
			var still []*sequence
			for _, seq := range running {
				if seq.prefillTicks > 0 {
					seq.prefillTicks--
					still = append(still, seq)
					continue
				}
				seq.decodeTicks--
				seq.generated = append(seq.generated, int32(5000+len(seq.generated)))
				if seq.decodeTicks <= 0 {
					tokensInFlight -= len(seq.req.PromptTokens) + int(seq.req.MaxNewTokens)
					seq.req.ResultCh <- Result{
						OutputTokens:  seq.generated,
						CacheHit:      seq.cacheHit,
						PrefillTokens: seq.prefillTokens,
					}
				} else {
					still = append(still, seq)
				}
			}
			running = still
		}
	}
}
