package torbox

import (
	"os"
	"strings"
)

func init() {
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
