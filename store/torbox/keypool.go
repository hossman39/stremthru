package torbox

import (
	"strings"
	"sync"
	"time"
)

type KeyHealth string

const (
	KeyHealthHealthy     KeyHealth = "healthy"
	KeyHealthRateLimited KeyHealth = "rate_limited"
	KeyHealthBlocked     KeyHealth = "blocked"
)

var (
	keyPoolRollingWindow   = 60 * time.Minute
	keyPoolRecoveryTimeout = 5 * time.Minute
)

type poolKey struct {
	key         string
	health      KeyHealth
	usageTimes  []time.Time
	lastErrorAt time.Time
}

func (k *poolKey) rollingUsage(now time.Time) int {
	cutoff := now.Add(-keyPoolRollingWindow)
	count := 0
	for _, t := range k.usageTimes {
		if t.After(cutoff) {
			count++
		}
	}
	return count
}

func (k *poolKey) pruneUsage(now time.Time) {
	cutoff := now.Add(-keyPoolRollingWindow)
	pruned := k.usageTimes[:0]
	for _, t := range k.usageTimes {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	k.usageTimes = pruned
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

type KeyPool struct {
	mu   sync.Mutex
	keys []*poolKey
}

func NewKeyPool(apiKeys []string) *KeyPool {
	var keys []*poolKey
	for _, k := range apiKeys {
		if k == "" {
			log.Warn("skipping empty key in pool")
			continue
		}
		keys = append(keys, &poolKey{
			key:        k,
			health:     KeyHealthHealthy,
			usageTimes: []time.Time{},
		})
	}
	log.Info("key pool initialized", "key_count", len(keys))
	return &KeyPool{keys: keys}
}

func (p *KeyPool) RecordUsage(apiKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for _, k := range p.keys {
		if k.key == apiKey {
			k.usageTimes = append(k.usageTimes, now)
			k.pruneUsage(now)
			return
		}
	}
}

func (p *KeyPool) RecordError(apiKey string, statusCode int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, k := range p.keys {
		if k.key == apiKey {
			switch statusCode {
			case 429:
				k.health = KeyHealthRateLimited
			case 401, 403:
				k.health = KeyHealthBlocked
			default:
				return
			}
			k.lastErrorAt = time.Now()
			log.Warn("key marked unhealthy", "key", maskKey(k.key), "status", k.health, "http_code", statusCode)
			return
		}
	}
}

func (p *KeyPool) GetKeyForRequest(incomingKey string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	// Find the incoming key in the pool
	var incomingPoolKey *poolKey
	for _, k := range p.keys {
		if k.key == incomingKey {
			incomingPoolKey = k
			break
		}
	}

	// Not a pool key - pass through unchanged
	if incomingPoolKey == nil {
		return incomingKey
	}

	// Try to use the incoming key if it's healthy (sticky preference)
	if incomingPoolKey.health == KeyHealthHealthy {
		return incomingPoolKey.key
	}

	// Incoming key is unhealthy - check if it should auto-recover
	if now.Sub(incomingPoolKey.lastErrorAt) >= keyPoolRecoveryTimeout {
		log.Info("key auto-recovered", "key", maskKey(incomingPoolKey.key), "previous_status", string(incomingPoolKey.health))
		incomingPoolKey.health = KeyHealthHealthy
		return incomingPoolKey.key
	}

	// Incoming key is unhealthy - find the best healthy alternative
	var best *poolKey
	bestUsage := -1
	for _, k := range p.keys {
		if k.key == incomingKey {
			continue
		}
		if k.health != KeyHealthHealthy {
			if now.Sub(k.lastErrorAt) >= keyPoolRecoveryTimeout {
				log.Info("key auto-recovered", "key", maskKey(k.key), "previous_status", string(k.health))
				k.health = KeyHealthHealthy
			} else {
				continue
			}
		}
		usage := k.rollingUsage(now)
		if best == nil || usage < bestUsage {
			best = k
			bestUsage = usage
		}
	}

	if best != nil {
		log.Info("using alternate key", "original", maskKey(incomingKey), "alternate", maskKey(best.key), "reason", string(incomingPoolKey.health))
		return best.key
	}

	// All keys unhealthy - use the incoming key anyway
	log.Warn("all keys unhealthy, using original key", "key", maskKey(incomingKey))
	return incomingKey
}

var creationPaths = []string{
	"/v1/api/torrents/createtorrent",
	"/v1/api/usenet/createusenetdownload",
	"/v1/api/webdl/createwebdownload",
}

var credentialValidationPaths = []string{
	"/v1/api/user/me",
}

func IsCreationPath(path string) bool {
	for _, p := range creationPaths {
		if strings.HasSuffix(path, p) {
			return true
		}
	}
	return false
}

func IsCredentialValidationPath(path string) bool {
	for _, p := range credentialValidationPaths {
		if strings.HasSuffix(path, p) {
			return true
		}
	}
	return false
}

var Pool *KeyPool
