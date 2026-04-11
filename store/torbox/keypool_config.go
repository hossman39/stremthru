package torbox

import (
	"os"
	"strings"
	"time"
)

func init() {
	if v := os.Getenv("STREMTHRU_TORBOX_POOL_ROLLING_WINDOW"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			keyPoolRollingWindow = d
		} else {
			log.Warn("invalid STREMTHRU_TORBOX_POOL_ROLLING_WINDOW, using default", "value", v, "default", keyPoolRollingWindow)
		}
	}

	if v := os.Getenv("STREMTHRU_TORBOX_POOL_RECOVERY_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			keyPoolRecoveryTimeout = d
		} else {
			log.Warn("invalid STREMTHRU_TORBOX_POOL_RECOVERY_TIMEOUT, using default", "value", v, "default", keyPoolRecoveryTimeout)
		}
	}

	poolKeys := os.Getenv("STREMTHRU_TORBOX_KEY_POOL")
	if poolKeys == "" {
		return
	}

	keys := []string{}
	for _, k := range strings.Split(poolKeys, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}

	if len(keys) == 0 {
		return
	}

	Pool = NewKeyPool(keys)
}
